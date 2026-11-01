package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/a1exus/ollametry/internal/telemetry"
)

const testModel = "granite4.2:8b"

type harness struct {
	t            *testing.T
	reader       *sdkmetric.ManualReader
	front        *httptest.Server
	upstreamHost string
}

func newHarness(t *testing.T, upstream http.Handler, maxInspect int) *harness {
	t.Helper()
	up := httptest.NewServer(upstream)
	t.Cleanup(up.Close)
	return newHarnessFor(t, up.URL, maxInspect)
}

func newHarnessFor(t *testing.T, upstreamURL string, maxInspect int) *harness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	in, err := telemetry.NewInstruments(mp.Meter("test"))
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}
	u, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	p, err := New(Config{
		Upstream:        u,
		MaxInspectBytes: maxInspect,
		Instruments:     in,
		Logger:          slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	front := httptest.NewServer(p)
	t.Cleanup(front.Close)
	return &harness{t: t, reader: reader, front: front, upstreamHost: u.Host}
}

func (h *harness) collect() metricdata.ResourceMetrics {
	h.t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &rm); err != nil {
		h.t.Fatalf("collect: %v", err)
	}
	return rm
}

// waitRecorded waits until the request has been fully recorded: the proxy
// records after the response body is relayed, so the client can be faster.
func (h *harness) waitRecorded() metricdata.ResourceMetrics {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rm := h.collect()
		var count uint64
		for _, dp := range histogram[float64](rm, "http.server.request.duration") {
			count += dp.Count
		}
		if count >= 1 {
			return rm
		}
		if time.Now().After(deadline) {
			h.t.Fatal("timed out waiting for the request to be recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *harness) post(path, body string) (*http.Response, []byte) {
	h.t.Helper()
	req, err := http.NewRequestWithContext(h.t.Context(), http.MethodPost, h.front.URL+path, strings.NewReader(body))
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read response: %v", err)
	}
	return resp, got
}

func histogram[N int64 | float64](rm metricdata.ResourceMetrics, name string) []metricdata.HistogramDataPoint[N] {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if h, ok := m.Data.(metricdata.Histogram[N]); ok {
					return h.DataPoints
				}
			}
		}
	}
	return nil
}

func sum(rm metricdata.ResourceMetrics, name string) []metricdata.DataPoint[int64] {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				if s, ok := m.Data.(metricdata.Sum[int64]); ok {
					return s.DataPoints
				}
			}
		}
	}
	return nil
}

func attr(set attribute.Set, key string) string {
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return v.String()
}

func hasAttr(set attribute.Set, key string) bool {
	_, ok := set.Value(attribute.Key(key))
	return ok
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "dialect", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

// replay serves body line by line, flushing after each, like Ollama does.
func replay(status int, contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		flusher, _ := w.(http.Flusher)
		for _, line := range bytes.SplitAfter(body, []byte("\n")) {
			_, _ = w.Write(line)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

func TestProxyRecordsRecordedResponses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		path        string
		fixture     string
		status      int
		contentType string
		operation   string

		wantErrorType     string
		wantHTTPErrorType string
		wantInput         int64
		wantOutput        int64
		wantCached        int64
		wantTTFT          bool
		wantTPOT          bool
	}{
		{
			name: "native chat stream", path: "/api/chat", fixture: "native_chat_stream.ndjson",
			status: 200, contentType: "application/x-ndjson", operation: "chat",
			wantInput: 21, wantOutput: 12, wantCached: 20, wantTTFT: true, wantTPOT: true,
		},
		{
			name: "native generate non-streaming", path: "/api/generate", fixture: "native_generate.json",
			status: 200, contentType: "application/json; charset=utf-8", operation: "text_completion",
			wantInput: 21, wantOutput: 12, wantCached: 20, wantTPOT: true,
		},
		{
			name: "native model not found", path: "/api/chat", fixture: "native_error_model.json",
			status: 404, contentType: "application/json; charset=utf-8", operation: "chat",
			wantErrorType: "404",
		},
		{
			name: "native embeddings unsupported", path: "/api/embed", fixture: "native_embed.json",
			status: 501, contentType: "application/json; charset=utf-8", operation: "embeddings",
			wantErrorType: "501", wantHTTPErrorType: "501",
		},
		{
			name: "openai stream without usage", path: "/v1/chat/completions", fixture: "openai_chat_stream.sse",
			status: 200, contentType: "text/event-stream", operation: "chat",
			wantTTFT: true,
		},
		{
			name: "openai stream with usage", path: "/v1/chat/completions", fixture: "openai_chat_stream_usage.sse",
			status: 200, contentType: "text/event-stream", operation: "chat",
			wantInput: 21, wantOutput: 12, wantCached: 20, wantTTFT: true, wantTPOT: true,
		},
		{
			name: "openai non-streaming", path: "/v1/chat/completions", fixture: "openai_chat.json",
			status: 200, contentType: "application/json", operation: "chat",
			wantInput: 21, wantOutput: 12, wantCached: 20,
		},
		{
			name: "anthropic stream", path: "/v1/messages", fixture: "anthropic_messages_stream.sse",
			status: 200, contentType: "text/event-stream", operation: "chat",
			wantInput: 21, wantOutput: 12, wantCached: 20, wantTTFT: true, wantTPOT: true,
		},
		{
			name: "anthropic model not found", path: "/v1/messages", fixture: "anthropic_error_model.json",
			status: 404, contentType: "application/json", operation: "chat",
			wantErrorType: "404",
		},
		{
			name: "responses stream", path: "/v1/responses", fixture: "responses_stream.sse",
			status: 200, contentType: "text/event-stream", operation: "chat",
			wantInput: 21, wantOutput: 12, wantCached: 20, wantTTFT: true, wantTPOT: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := fixture(t, tt.fixture)
			h := newHarness(t, replay(tt.status, tt.contentType, body), 1<<20)

			resp, got := h.post(tt.path, `{"model":"`+testModel+`","stream":true}`)
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}
			if !bytes.Equal(got, body) {
				t.Fatalf("relayed body differs from upstream body (%d vs %d bytes)", len(got), len(body))
			}
			rm := h.waitRecorded()

			httpDP := histogram[float64](rm, "http.server.request.duration")
			if len(httpDP) != 1 {
				t.Fatalf("http.server.request.duration: %d data points, want 1", len(httpDP))
			}
			if got := attr(httpDP[0].Attributes, "http.route"); got != tt.path {
				t.Errorf("http.route = %q, want %q", got, tt.path)
			}
			if got := attr(httpDP[0].Attributes, "error.type"); got != tt.wantHTTPErrorType {
				t.Errorf("http error.type = %q, want %q", got, tt.wantHTTPErrorType)
			}

			op := histogram[float64](rm, "gen_ai.client.operation.duration")
			if len(op) != 1 || op[0].Count != 1 {
				t.Fatalf("gen_ai.client.operation.duration = %+v, want one observation", op)
			}
			for key, want := range map[string]string{
				"gen_ai.operation.name": tt.operation,
				"gen_ai.provider.name":  "ollama",
				"gen_ai.request.model":  testModel,
				"error.type":            tt.wantErrorType,
				"server.address":        "127.0.0.1",
			} {
				if got := attr(op[0].Attributes, key); got != want {
					t.Errorf("operation duration %s = %q, want %q", key, got, want)
				}
			}

			tokens := map[string]int64{}
			for _, dp := range histogram[int64](rm, "gen_ai.client.token.usage") {
				tokens[attr(dp.Attributes, "gen_ai.token.type")] += dp.Sum
				if got := attr(dp.Attributes, "gen_ai.response.model"); got != testModel {
					t.Errorf("token usage gen_ai.response.model = %q, want %q", got, testModel)
				}
			}
			if tokens["input"] != tt.wantInput || tokens["output"] != tt.wantOutput {
				t.Errorf("tokens = %v, want input %d output %d", tokens, tt.wantInput, tt.wantOutput)
			}
			var cached int64
			for _, dp := range sum(rm, "ollama.prompt.cached_tokens") {
				cached += dp.Value
			}
			if cached != tt.wantCached {
				t.Errorf("cached tokens = %d, want %d", cached, tt.wantCached)
			}
			if got := len(histogram[float64](rm, "gen_ai.server.time_to_first_token")) == 1; got != tt.wantTTFT {
				t.Errorf("time to first token recorded = %v, want %v", got, tt.wantTTFT)
			}
			if got := len(histogram[float64](rm, "gen_ai.server.time_per_output_token")) == 1; got != tt.wantTPOT {
				t.Errorf("time per output token recorded = %v, want %v", got, tt.wantTPOT)
			}
			for _, dp := range sum(rm, "ollama.inference.active") {
				if dp.Value != 0 {
					t.Errorf("ollama.inference.active = %d after the request, want 0", dp.Value)
				}
			}
		})
	}
}

func TestProxyTimePerOutputTokenUsesServerTiming(t *testing.T) {
	t.Parallel()
	h := newHarness(t, replay(200, "application/x-ndjson", fixture(t, "native_chat_stream.ndjson")), 1<<20)
	h.post("/api/chat", `{"model":"`+testModel+`"}`)
	rm := h.waitRecorded()

	tpot := histogram[float64](rm, "gen_ai.server.time_per_output_token")
	if len(tpot) != 1 {
		t.Fatalf("time per output token: %d data points, want 1", len(tpot))
	}
	// eval_duration 820472000 ns / eval_count 12.
	if want := 0.820472 / 12; abs(tpot[0].Sum-want) > 1e-9 {
		t.Errorf("time per output token = %v s, want %v s", tpot[0].Sum, want)
	}
	load := histogram[float64](rm, "ollama.model.load.duration")
	if len(load) != 1 || abs(load[0].Sum-0.004588709) > 1e-9 {
		t.Errorf("ollama.model.load.duration = %+v, want one observation of 0.004588709 s", load)
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func TestProxyStreamsWithoutBuffering(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"model":"m","response":"Hi","done":false}`+"\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, `{"model":"m","response":"","done":true,"prompt_eval_count":3,"eval_count":1,"eval_duration":1000}`+"\n")
	})
	h := newHarness(t, upstream, 1<<20)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.front.URL+"/api/generate", strings.NewReader(`{"model":"m"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	first := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.Body).ReadString('\n')
		first <- line
	}()
	select {
	case line := <-first:
		if !strings.Contains(line, `"Hi"`) {
			t.Errorf("first line = %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first chunk not delivered while upstream was still generating: response is buffered")
	}
	close(release)
}

func TestProxyClientCancelStopsUpstream(t *testing.T) {
	t.Parallel()
	upstreamCanceled := make(chan struct{})
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, `{"model":"m","response":"Hi","done":false}`+"\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamCanceled)
	})
	h := newHarness(t, upstream, 1<<20)

	ctx, cancel := context.WithCancel(t.Context())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, h.front.URL+"/api/chat", strings.NewReader(`{"model":"m"}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatalf("read first line: %v", err)
	}
	cancel()
	_ = resp.Body.Close()

	select {
	case <-upstreamCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request was not canceled after the client hung up: Ollama would keep generating")
	}
	rm := h.waitRecorded()
	op := histogram[float64](rm, "gen_ai.client.operation.duration")
	if len(op) != 1 || attr(op[0].Attributes, "error.type") != ErrorTypeCanceled {
		t.Errorf("operation duration = %+v, want error.type %q", op, ErrorTypeCanceled)
	}
	if ttft := histogram[float64](rm, "gen_ai.server.time_to_first_token"); len(ttft) != 1 {
		t.Errorf("time to first token not recorded for a canceled stream that produced a token")
	}
}

func TestProxyTruncatedStreamIsIncomplete(t *testing.T) {
	t.Parallel()
	upstream := replay(200, "application/x-ndjson", []byte(`{"model":"m","response":"Hi","done":false}`+"\n"))
	h := newHarness(t, upstream, 1<<20)
	h.post("/api/chat", `{"model":"m"}`)
	rm := h.waitRecorded()

	op := histogram[float64](rm, "gen_ai.client.operation.duration")
	if len(op) != 1 || attr(op[0].Attributes, "error.type") != ErrorTypeIncomplete {
		t.Errorf("operation duration = %+v, want error.type %q", op, ErrorTypeIncomplete)
	}
	httpDP := histogram[float64](rm, "http.server.request.duration")
	if len(httpDP) != 1 || attr(httpDP[0].Attributes, "error.type") != ErrorTypeIncomplete {
		t.Errorf("http duration = %+v, want error.type %q", httpDP, ErrorTypeIncomplete)
	}
}

func TestProxyUpstreamUnavailable(t *testing.T) {
	t.Parallel()
	// Reserve a port, then close it so nothing listens there.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	h := newHarnessFor(t, "http://"+addr, 1<<20)
	resp, body := h.post("/api/chat", `{"model":"m"}`)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if !strings.Contains(string(body), `"error"`) {
		t.Errorf("body = %s, want an Ollama-style error object", body)
	}
	rm := h.waitRecorded()
	httpDP := histogram[float64](rm, "http.server.request.duration")
	if len(httpDP) != 1 || attr(httpDP[0].Attributes, "error.type") != ErrorTypeUpstream ||
		attr(httpDP[0].Attributes, "http.response.status_code") != "502" {
		t.Errorf("http duration attributes = %v, want status 502 and error.type %q", httpDP, ErrorTypeUpstream)
	}
}

func TestProxyRelaysRequestUnchanged(t *testing.T) {
	t.Parallel()
	type seen struct {
		path, host, forwardedFor string
		body                     []byte
	}
	got := make(chan seen, 1)
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- seen{r.URL.RequestURI(), r.Host, r.Header.Get("X-Forwarded-For"), b}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})
	const maxInspect = 64
	tests := []struct {
		name             string
		path             string
		body             string
		wantGenAI        bool
		wantRequestModel string
	}{
		{
			name: "blob upload larger than the inspect limit is not inspected",
			path: "/api/blobs/sha256:abc", body: strings.Repeat("x", 10*maxInspect),
		},
		{
			name: "oversized inference body still yields the model from its prefix",
			path: "/api/chat?x=1", body: `{"model":"big","messages":[{"content":"` + strings.Repeat("y", 10*maxInspect) + `"}]}`,
			wantGenAI: true, wantRequestModel: "big",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, upstream, maxInspect)
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.front.URL+tt.path, strings.NewReader(tt.body))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			_ = resp.Body.Close()

			s := <-got
			if s.path != tt.path {
				t.Errorf("upstream path = %q, want %q", s.path, tt.path)
			}
			if string(s.body) != tt.body {
				t.Errorf("upstream body differs: %d bytes, want %d", len(s.body), len(tt.body))
			}
			if s.host != h.upstreamHost {
				t.Errorf("upstream Host = %q, want %q", s.host, h.upstreamHost)
			}
			if s.forwardedFor == "" {
				t.Error("X-Forwarded-For not set")
			}
			rm := h.waitRecorded()
			op := histogram[float64](rm, "gen_ai.client.operation.duration")
			if (len(op) > 0) != tt.wantGenAI {
				t.Fatalf("gen_ai metrics recorded = %v, want %v", len(op) > 0, tt.wantGenAI)
			}
			if tt.wantGenAI && attr(op[0].Attributes, "gen_ai.request.model") != tt.wantRequestModel {
				t.Errorf("gen_ai.request.model = %q, want %q", attr(op[0].Attributes, "gen_ai.request.model"), tt.wantRequestModel)
			}
			httpDP := histogram[float64](rm, "http.server.request.duration")
			if hasAttr(httpDP[0].Attributes, "server.address") {
				t.Error("http.server.request.duration carries server.address of the upstream")
			}
		})
	}
}

func TestProxyUnknownRouteHasNoRouteAttribute(t *testing.T) {
	t.Parallel()
	h := newHarness(t, http.NotFoundHandler(), 1<<20)
	h.post("/some/"+strings.Repeat("random", 3), `{}`)
	rm := h.waitRecorded()
	httpDP := histogram[float64](rm, "http.server.request.duration")
	if len(httpDP) != 1 || hasAttr(httpDP[0].Attributes, "http.route") {
		t.Errorf("http duration = %+v, want one point without http.route", httpDP)
	}
	if op := histogram[float64](rm, "gen_ai.client.operation.duration"); len(op) != 0 {
		t.Errorf("gen_ai metrics recorded for a non-inference route")
	}
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	mp := sdkmetric.NewMeterProvider()
	in, err := telemetry.NewInstruments(mp.Meter("test"))
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}
	logger := slog.New(slog.DiscardHandler)
	mustURL := func(s string) *url.URL {
		u, _ := url.Parse(s)
		return u
	}
	tests := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"valid", Config{Upstream: mustURL("http://127.0.0.1:11434"), MaxInspectBytes: 1, Instruments: in, Logger: logger}, true},
		{"no upstream", Config{MaxInspectBytes: 1, Instruments: in, Logger: logger}, false},
		{"bad scheme", Config{Upstream: mustURL("ftp://x"), MaxInspectBytes: 1, Instruments: in, Logger: logger}, false},
		{"zero inspect", Config{Upstream: mustURL("http://x"), Instruments: in, Logger: logger}, false},
		{"no instruments", Config{Upstream: mustURL("http://x"), MaxInspectBytes: 1, Logger: logger}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tt.cfg)
			if (err == nil) != tt.ok {
				t.Errorf("New() error = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestModelFromPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		prefix, want string
	}{
		{`{"model":"a","messages":[`, "a"},
		{`{"stream":true,"options":{"x":[1,2]},"model":"b","messa`, "b"},
		{`{"messages":[{"content":"trunc`, ""},
		{`[1,2]`, ""},
		{``, ""},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			if got := modelFromPrefix([]byte(tt.prefix)); got != tt.want {
				t.Errorf("modelFromPrefix(%q) = %q, want %q", tt.prefix, got, tt.want)
			}
		})
	}
}

// An upstream that compresses when asked must not hide usage from the proxy,
// and the client must still get a body it can read.
func TestProxyDecodesCompressedUpstream(t *testing.T) {
	t.Parallel()
	plain := fixture(t, "native_chat.json")
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = w.Write(plain)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = zw.Write(plain)
		_ = zw.Close()
	})
	h := newHarness(t, upstream, 1<<20)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.front.URL+"/api/chat", strings.NewReader(`{"model":"`+testModel+`"}`))
	req.Header.Set("Accept-Encoding", "gzip") // set by hand, so net/http will not decompress for us
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if enc := resp.Header.Get("Content-Encoding"); enc != "" || !bytes.Equal(got, plain) {
		t.Errorf("client got Content-Encoding %q and %d bytes, want an uncompressed copy of the %d-byte body", enc, len(got), len(plain))
	}

	rm := h.waitRecorded()
	var output int64
	for _, dp := range histogram[int64](rm, "gen_ai.client.token.usage") {
		if attr(dp.Attributes, "gen_ai.token.type") == "output" {
			output += dp.Sum
		}
	}
	if output != 12 {
		t.Errorf("output tokens = %d, want 12 decoded from the compressed response", output)
	}
}
