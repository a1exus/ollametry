// Package proxy is a transparent reverse proxy in front of Ollama that
// records OpenTelemetry metrics for every request it relays.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"

	"github.com/a1exus/ollametry/internal/dialect"
	"github.com/a1exus/ollametry/internal/genai"
	"github.com/a1exus/ollametry/internal/route"
	"github.com/a1exus/ollametry/internal/telemetry"
)

// maxModelNameLen bounds gen_ai.request.model; longer values are not model
// names and would only add cardinality.
const maxModelNameLen = 256

// otherModel replaces model names that are too long or unknown to Ollama.
const otherModel = "_OTHER"

// Config configures a Proxy.
type Config struct {
	// Upstream is Ollama's base URL.
	Upstream *url.URL
	// MaxInspectBytes bounds both the request body buffered to read the model
	// name and each response document decoded. Larger bodies are relayed
	// untouched; only their metrics are partial.
	MaxInspectBytes int
	Instruments     *telemetry.Instruments
	Logger          *slog.Logger
	// Transport defaults to NewTransport().
	Transport http.RoundTripper
	// KnownModel reports whether a requested model exists; names it rejects
	// are recorded as "_OTHER" so that typos and probes cannot fill the
	// cardinality limit. Nil accepts every name.
	KnownModel func(model string) bool
}

// Proxy is an http.Handler relaying requests to Ollama.
type Proxy struct {
	cfg        Config
	rp         *httputil.ReverseProxy
	serverAttr []attribute.KeyValue
	// inflight counts running handlers so shutdown can wait for their
	// metrics, which http.Server.Close does not.
	inflight sync.WaitGroup
}

// New validates cfg and builds a Proxy.
func New(cfg Config) (*Proxy, error) {
	switch {
	case cfg.Upstream == nil || cfg.Upstream.Host == "":
		return nil, errors.New("proxy: upstream URL with a host is required")
	case cfg.Upstream.Scheme != "http" && cfg.Upstream.Scheme != "https":
		return nil, fmt.Errorf("proxy: upstream scheme %q is not http or https", cfg.Upstream.Scheme)
	case cfg.MaxInspectBytes <= 0:
		return nil, fmt.Errorf("proxy: MaxInspectBytes must be positive, got %d", cfg.MaxInspectBytes)
	case cfg.Instruments == nil || cfg.Logger == nil:
		return nil, errors.New("proxy: Instruments and Logger are required")
	}
	if cfg.Transport == nil {
		cfg.Transport = NewTransport()
	}

	p := &Proxy{cfg: cfg, serverAttr: serverAttributes(cfg.Upstream)}
	p.rp = &httputil.ReverseProxy{
		Rewrite:        p.rewrite,
		Transport:      cfg.Transport,
		FlushInterval:  -1, // flush every write: tokens must reach clients as they are generated
		BufferPool:     &copyBuffers,
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.errorHandler,
		// Copy errors are mostly clients hanging up; they are already
		// reflected in error.type, so keep them out of the default log.
		ErrorLog: slog.NewLogLogger(cfg.Logger.Handler(), slog.LevelDebug),
	}
	return p, nil
}

// copyBuffers reuses ReverseProxy's 32 KiB copy buffers; without a pool it
// allocates one per request.
var copyBuffers bufferPool

type bufferPool struct{ pool sync.Pool }

func (b *bufferPool) Get() []byte {
	if buf, ok := b.pool.Get().(*[]byte); ok {
		return *buf
	}
	return make([]byte, 32<<10)
}

func (b *bufferPool) Put(buf []byte) {
	b.pool.Put(&buf)
}

// NewTransport returns the upstream transport. It ignores HTTP(S)_PROXY, has
// no response header timeout because a non-streaming generation sends its
// headers only when it is done, and keeps enough idle connections for
// parallel requests to a local server.
func NewTransport() *http.Transport {
	return &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.inflight.Add(1)
	defer p.inflight.Done()

	ex := &exchange{
		start:  time.Now(),
		method: r.Method,
		route:  route.Match(r.Method, r.URL.Path),
	}
	// Metrics are recorded after the client may have gone; keep the
	// request's values but not its cancellation.
	mctx := context.WithoutCancel(r.Context())

	activeAttrs := p.httpActiveAttrs(ex)
	p.cfg.Instruments.HTTPActive.Add(mctx, 1, methodAttr(r.Method), "http", activeAttrs...)
	defer p.cfg.Instruments.HTTPActive.Add(mctx, -1, methodAttr(r.Method), "http", activeAttrs...)

	if ex.route.Inference() {
		model, err := p.inspectRequest(r)
		if err != nil {
			ex.status = http.StatusBadRequest
			p.finish(mctx, ex, r.Context().Err())
			writeError(w, http.StatusBadRequest, "read request body: "+err.Error())
			return
		}
		ex.loggedModel = model
		if model != "" && p.cfg.KnownModel != nil && !p.cfg.KnownModel(model) {
			model = otherModel
		}
		ex.requestModel = model
		inflight := metric.WithAttributes(p.genAIAttrs(ex, "")...)
		p.cfg.Instruments.InferenceActive.Add(mctx, 1, inflight)
		defer p.cfg.Instruments.InferenceActive.Add(mctx, -1, inflight)
	}

	defer func() {
		recovered := recover()
		if errors.Is(asError(recovered), http.ErrAbortHandler) {
			ex.aborted = true
		}
		p.finish(mctx, ex, r.Context().Err())
		if recovered != nil {
			panic(recovered) // net/http handles http.ErrAbortHandler by closing the connection.
		}
	}()
	p.rp.ServeHTTP(w, r.WithContext(withExchange(r.Context(), ex)))
}

// Wait blocks until every handler has recorded its metrics or ctx is done.
// Call it after the server stopped accepting requests, before flushing the
// meter provider.
func (p *Proxy) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		p.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for in-flight requests: %w", ctx.Err())
	}
}

func asError(v any) error {
	err, _ := v.(error)
	return err
}

func (p *Proxy) rewrite(pr *httputil.ProxyRequest) {
	pr.SetURL(p.cfg.Upstream)
	pr.SetXForwarded()
	if ex := exchangeFrom(pr.In.Context()); ex != nil && ex.route.Inference() {
		// Without a client Accept-Encoding the transport negotiates gzip
		// itself and decompresses transparently, so the body tap always sees
		// plain JSON whatever the client asked for.
		pr.Out.Header.Del("Accept-Encoding")
	}
}

func (p *Proxy) modifyResponse(resp *http.Response) error {
	ex := exchangeFrom(resp.Request.Context())
	if ex == nil {
		return nil
	}
	ex.status = resp.StatusCode
	if resp.StatusCode == http.StatusSwitchingProtocols {
		// ReverseProxy needs the raw, writable body to relay an upgrade.
		return nil
	}
	tap := &tapBody{ReadCloser: resp.Body, ex: ex}
	if ex.route.Inference() {
		ex.format = dialect.FormatFromContentType(resp.Header.Get("Content-Type"))
		if enc := resp.Header.Get("Content-Encoding"); enc != "" && enc != "identity" {
			ex.format = dialect.FormatUnknown
		}
		if dec := dialect.NewDecoder(ex.route.Dialect); dec != nil && ex.format != dialect.FormatUnknown {
			tap.framer = dialect.NewFramer(ex.format, p.cfg.MaxInspectBytes, func(doc []byte) {
				dec.Decode(doc, time.Now(), &ex.result)
			})
		}
	}
	resp.Body = tap
	return nil
}

func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	ex := exchangeFrom(r.Context())
	if ex != nil {
		ex.transportErr = err
	}
	if r.Context().Err() != nil {
		return // the client is gone; nobody to answer
	}
	if ex != nil {
		ex.status = http.StatusBadGateway
	}
	p.cfg.Logger.LogAttrs(r.Context(), slog.LevelWarn, "upstream request failed",
		slog.String("method", logMethod(r.Method)), slog.String("path", logPath(r.URL.Path)), slog.Any("error", err))
	writeError(w, http.StatusBadGateway, "ollametry: upstream unavailable: "+err.Error())
}

// writeError answers in Ollama's native error shape.
func writeError(w http.ResponseWriter, status int, msg string) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// inspectRequest reads the model name from an inference request body and
// replaces the body with an identical one. Bodies over MaxInspectBytes are
// relayed as a buffered prefix followed by the unread remainder.
func (p *Proxy) inspectRequest(r *http.Request) (string, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return "", nil
	}
	limit := int64(p.cfg.MaxInspectBytes)
	buf, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return "", fmt.Errorf("buffer up to %d bytes: %w", limit, err)
	}
	if int64(len(buf)) <= limit {
		r.Body = io.NopCloser(bytes.NewReader(buf))
		return sanitizeModel(modelFromBody(buf)), nil
	}
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(buf), r.Body), r.Body}
	return sanitizeModel(modelFromPrefix(buf)), nil
}

func modelFromBody(body []byte) string {
	var v struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return ""
	}
	return v.Model
}

// modelFromPrefix walks the top-level members of a truncated JSON object and
// returns the model it names so far. It resolves keys the way encoding/json
// does, and with it Ollama: case-insensitively, last duplicate wins, null
// leaves the value unchanged, and a non-string model invalidates the body.
func modelFromPrefix(prefix []byte) string {
	dec := json.NewDecoder(bytes.NewReader(prefix))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return ""
	}
	var model string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return model
		}
		if key, _ := tok.(string); strings.EqualFold(key, "model") {
			if err := dec.Decode(&model); err != nil {
				return ""
			}
			continue
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return model
		}
	}
	return model
}

func sanitizeModel(model string) string {
	if len(model) > maxModelNameLen {
		return otherModel
	}
	return model
}

// finish records every metric and the access log line for an exchange.
func (p *Proxy) finish(ctx context.Context, ex *exchange, clientErr error) {
	elapsed := time.Since(ex.start)
	errType := ex.errorType(clientErr)
	in := p.cfg.Instruments

	httpAttrs := p.httpActiveAttrs(ex)
	if ex.status > 0 {
		httpAttrs = append(httpAttrs, in.HTTPDuration.AttrResponseStatusCode(ex.status))
	}
	// For server spans and metrics a 4xx is the client's error, not ours.
	if errType != "" && (ex.status < 400 || ex.status >= 500) {
		httpAttrs = append(httpAttrs, in.HTTPDuration.AttrErrorType(httpconv.ErrorTypeAttr(errType)))
	}
	in.HTTPDuration.Record(ctx, elapsed.Seconds(), methodAttr(ex.method), "http", httpAttrs...)

	if !ex.route.Inference() {
		p.cfg.Logger.LogAttrs(ctx, slog.LevelDebug, "request",
			slog.String("method", logMethod(ex.method)), slog.String("route", ex.route.Pattern),
			slog.Int("status", ex.status), slog.Duration("duration", elapsed), slog.String("error.type", errType))
		return
	}

	r := &ex.result
	base := p.genAIAttrs(ex, r.ResponseModel)
	opAttrs := base
	if errType != "" {
		opAttrs = append(opAttrs[:len(opAttrs):len(opAttrs)], attribute.String("error.type", errType))
	}
	in.OperationDuration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(opAttrs...))

	var ttft time.Duration
	if ex.format.Streaming() && !r.FirstTokenAt.IsZero() {
		ttft = r.FirstTokenAt.Sub(ex.start)
		in.TimeToFirstToken.Record(ctx, ttft.Seconds(), metric.WithAttributes(base...))
	}
	if errType == "" && r.TokensReported {
		in.TokenUsage.Record(ctx, r.InputTokens, metric.WithAttributes(withTokenType(base, genai.TokenTypeInput)...))
		if ex.route.Operation != genai.OperationEmbeddings {
			in.TokenUsage.Record(ctx, r.OutputTokens, metric.WithAttributes(withTokenType(base, genai.TokenTypeOutput)...))
		}
		if r.CachedInputTokens > 0 {
			in.CachedInputTokens.Add(ctx, r.CachedInputTokens, metric.WithAttributes(base...))
		}
		if tpot, ok := ex.timePerOutputToken(); ok {
			in.TimePerOutputToken.Record(ctx, tpot.Seconds(), metric.WithAttributes(base...))
		}
	}
	if r.LoadDuration > 0 {
		in.ModelLoadDuration.Record(ctx, r.LoadDuration.Seconds(), metric.WithAttributes(base...))
	}
	if r.PromptEvalDuration > 0 {
		in.PromptEvalDuration.Record(ctx, r.PromptEvalDuration.Seconds(), metric.WithAttributes(base...))
	}

	level := slog.LevelInfo
	if errType != "" {
		level = slog.LevelWarn
	}
	p.cfg.Logger.LogAttrs(ctx, level, "inference",
		slog.String("route", ex.route.Pattern),
		slog.String("model", ex.loggedModel),
		slog.Int("status", ex.status),
		slog.Duration("duration", elapsed),
		slog.Duration("ttft", ttft),
		slog.Int64("input_tokens", r.InputTokens),
		slog.Int64("cached_input_tokens", r.CachedInputTokens),
		slog.Int64("output_tokens", r.OutputTokens),
		slog.String("finish_reason", r.FinishReason),
		slog.String("error.type", errType),
	)
}

// httpActiveAttrs are the optional http.server.* attributes. server.address is
// left out on purpose: there it means this proxy, not Ollama.
func (p *Proxy) httpActiveAttrs(ex *exchange) []attribute.KeyValue {
	if ex.route.Pattern == "" {
		return nil
	}
	return []attribute.KeyValue{attribute.String("http.route", ex.route.Pattern)}
}

// genAIAttrs are the attributes shared by every gen_ai.* measurement.
func (p *Proxy) genAIAttrs(ex *exchange, responseModel string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 8)
	attrs = append(attrs,
		genai.KeyOperationName.String(ex.route.Operation),
		genai.KeyProviderName.String(genai.ProviderOllama),
		attribute.String("http.route", ex.route.Pattern),
	)
	attrs = append(attrs, p.serverAttr...)
	if ex.requestModel != "" {
		attrs = append(attrs, genai.KeyRequestModel.String(ex.requestModel))
	}
	if responseModel != "" {
		attrs = append(attrs, genai.KeyResponseModel.String(sanitizeModel(responseModel)))
	}
	return attrs
}

func withTokenType(base []attribute.KeyValue, tokenType string) []attribute.KeyValue {
	return append(base[:len(base):len(base)], genai.KeyTokenType.String(tokenType))
}

func serverAttributes(u *url.URL) []attribute.KeyValue {
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	attrs := []attribute.KeyValue{attribute.String("server.address", u.Hostname())}
	if n, err := strconv.Atoi(port); err == nil {
		attrs = append(attrs, attribute.Int("server.port", n))
	}
	return attrs
}

var knownMethods = map[string]httpconv.RequestMethodAttr{
	http.MethodConnect: httpconv.RequestMethodConnect,
	http.MethodDelete:  httpconv.RequestMethodDelete,
	http.MethodGet:     httpconv.RequestMethodGet,
	http.MethodHead:    httpconv.RequestMethodHead,
	http.MethodOptions: httpconv.RequestMethodOptions,
	http.MethodPatch:   httpconv.RequestMethodPatch,
	http.MethodPost:    httpconv.RequestMethodPost,
	http.MethodPut:     httpconv.RequestMethodPut,
	http.MethodTrace:   httpconv.RequestMethodTrace,
}

func methodAttr(method string) httpconv.RequestMethodAttr {
	if m, ok := knownMethods[method]; ok {
		return m
	}
	return httpconv.RequestMethodOther
}

// maxLoggedPathLen bounds the request path in log lines.
const maxLoggedPathLen = 256

// logMethod and logPath bound client-controlled values before they reach a
// log line. slog's handlers already escape control characters, so lines
// cannot be forged; this keeps a client from inflating each line up to the
// 1 MB header limit. Model names are bounded by sanitizeModel.
func logMethod(method string) string {
	return string(methodAttr(method))
}

func logPath(path string) string {
	if len(path) <= maxLoggedPathLen {
		return path
	}
	return strings.ToValidUTF8(path[:maxLoggedPathLen], "") + "…"
}
