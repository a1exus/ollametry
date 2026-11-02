//go:build integration

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

const testModel = "granite4.2:8b"

// fakeOllama replays responses recorded from Ollama 0.40.13.
func fakeOllama(t *testing.T) *httptest.Server {
	t.Helper()
	routes := map[string]struct{ file, contentType string }{
		"/api/version": {"../../internal/ollama/testdata/api_version.json", "application/json; charset=utf-8"},
		"/api/ps":      {"../../internal/ollama/testdata/api_ps.json", "application/json; charset=utf-8"},
		"/api/tags":    {"../../internal/ollama/testdata/api_tags.json", "application/json; charset=utf-8"},
		"/api/chat":    {"../../internal/dialect/testdata/native_chat_stream.ndjson", "application/x-ndjson"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", route.contentType)
		_, _ = w.Write(readFile(t, route.file))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func readFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		t.Errorf("read %s: %v", name, err)
	}
	return b
}

// collector is a fake OTLP/HTTP metrics receiver that decodes what it gets.
type collector struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*colmetricpb.ExportMetricsServiceRequest
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/metrics" {
			t.Errorf("collector got %s %s, want POST /v1/metrics", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read export: %v", err)
			return
		}
		var req colmetricpb.ExportMetricsServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			t.Errorf("decode export (Content-Type %q): %v", r.Header.Get("Content-Type"), err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.requests = append(c.requests, &req)
		c.mu.Unlock()
		resp, _ := proto.Marshal(&colmetricpb.ExportMetricsServiceResponse{})
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(resp)
	}))
	t.Cleanup(c.Close)
	return c
}

// exported is one metric as the last export reported it, with its resource.
type exported struct {
	resource []*commonpb.KeyValue
	metric   *metricpb.Metric
}

// last returns every metric of the most recent export containing it.
// Temporality is cumulative, so the last export holds the totals.
func (c *collector) last() map[string]exported {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]exported{}
	for _, req := range c.requests {
		for _, rm := range req.GetResourceMetrics() {
			for _, sm := range rm.GetScopeMetrics() {
				for _, m := range sm.GetMetrics() {
					out[m.GetName()] = exported{resource: rm.GetResource().GetAttributes(), metric: m}
				}
			}
		}
	}
	return out
}

func attrValue(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.GetKey() == key {
			return kv.GetValue().GetStringValue()
		}
	}
	return ""
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release port %s: %v", addr, err)
	}
	return addr
}

func waitReady(t *testing.T, url string, done <-chan error) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("run exited before becoming ready: %v", err)
		default:
		}
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s not ready within 10s", url)
}

// The whole binary: config from the environment, poller, proxy and the OTLP
// exporter, against a fake Ollama and a fake collector. Not parallel: the
// exporter reads OTEL_* from the process environment.
func TestRunExportsMetricsOverOTLP(t *testing.T) {
	ollama := fakeOllama(t)
	coll := newCollector(t)
	t.Setenv("OTEL_METRICS_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", coll.URL)
	t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	// No periodic export during the test: only the flush at shutdown runs,
	// which is also what this test checks.
	t.Setenv("OTEL_METRIC_EXPORT_INTERVAL", "3600000")

	proxyAddr, adminAddr := freeAddr(t), freeAddr(t)
	env := map[string]string{
		"OLLAMETRY_OLLAMA_URL":       ollama.URL,
		"OLLAMETRY_LISTEN_ADDR":      proxyAddr,
		"OLLAMETRY_ADMIN_ADDR":       adminAddr,
		"OLLAMETRY_POLL_INTERVAL":    "50ms",
		"OLLAMETRY_SHUTDOWN_TIMEOUT": "5s",
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var logs safeBuffer
	done := make(chan error, 1)
	go func() { done <- run(ctx, nil, func(k string) string { return env[k] }, &logs, io.Discard) }()
	waitReady(t, "http://"+adminAddr+"/readyz", done)

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+proxyAddr+"/api/chat",
		strings.NewReader(`{"model":"`+testModel+`","messages":[{"role":"user","content":"hi"}]}`))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/chat through the proxy: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if want := readFile(t, "../../internal/dialect/testdata/native_chat_stream.ndjson"); !bytes.Equal(got, want) {
		t.Errorf("proxy changed the response body: got %d bytes, want the %d recorded", len(got), len(want))
	}

	t.Run("prometheus endpoint", func(t *testing.T) {
		// Recording happens after the response body is relayed; poll.
		var scrape string
		deadline := time.Now().Add(5 * time.Second)
		for {
			req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+adminAddr+"/metrics", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET /metrics: %v", err)
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			scrape = string(b)
			if strings.Contains(scrape, "gen_ai_client_token_usage_sum") || time.Now().After(deadline) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		for _, want := range []string{
			"\nollama_up 1\n",
			`gen_ai_request_model="` + testModel + `"`,
			`gen_ai_token_type="output",http_route="/api/chat"`,
		} {
			if !strings.Contains(scrape, want) {
				t.Errorf("/metrics lacks %q", want)
			}
		}
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v on shutdown, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run did not return within 15s of cancellation")
	}

	metrics := coll.last()
	if len(metrics) == 0 {
		t.Fatalf("collector received no metrics; logs:\n%s", logs.String())
	}
	t.Run("resource", func(t *testing.T) {
		res := metrics["ollama.up"].resource
		for key, want := range map[string]string{"service.name": "ollametry", "service.version": "dev"} {
			if got := attrValue(res, key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
	})
	t.Run("ollama.up", func(t *testing.T) {
		points := metrics["ollama.up"].metric.GetGauge().GetDataPoints()
		if len(points) != 1 || points[0].GetAsInt() != 1 {
			t.Errorf("ollama.up = %v, want one point of 1", points)
		}
	})
	t.Run("operation duration", func(t *testing.T) {
		points := metrics["gen_ai.client.operation.duration"].metric.GetHistogram().GetDataPoints()
		if len(points) != 1 || points[0].GetCount() != 1 {
			t.Fatalf("operation duration points = %v, want one with count 1", points)
		}
		for key, want := range map[string]string{
			"gen_ai.request.model":  testModel,
			"gen_ai.response.model": testModel,
			"gen_ai.operation.name": "chat",
			"gen_ai.provider.name":  "ollama",
			"http.route":            "/api/chat",
		} {
			if got := attrValue(points[0].GetAttributes(), key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
	})
	t.Run("token usage", func(t *testing.T) {
		tokens := map[string]float64{}
		for _, p := range metrics["gen_ai.client.token.usage"].metric.GetHistogram().GetDataPoints() {
			tokens[attrValue(p.GetAttributes(), "gen_ai.token.type")] += p.GetSum()
		}
		if tokens["input"] != 21 || tokens["output"] != 12 {
			t.Errorf("token usage = %v, want input 21 and output 12 (from the recorded stream)", tokens)
		}
	})
	t.Run("time to first token", func(t *testing.T) {
		if n := len(metrics["gen_ai.server.time_to_first_token"].metric.GetHistogram().GetDataPoints()); n != 1 {
			t.Errorf("time to first token has %d points, want 1", n)
		}
	})
	t.Run("http duration", func(t *testing.T) {
		if len(metrics["http.server.request.duration"].metric.GetHistogram().GetDataPoints()) == 0 {
			t.Error("http.server.request.duration not exported")
		}
	})
	if !strings.Contains(logs.String(), `"msg":"ollametry stopped"`) {
		t.Errorf("no shutdown log line; logs:\n%s", logs.String())
	}
}

func TestRunFailsWhenProxyPortIsTaken(t *testing.T) {
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	taken, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer taken.Close()

	env := map[string]string{
		"OLLAMETRY_OLLAMA_URL":  "http://127.0.0.1:1",
		"OLLAMETRY_LISTEN_ADDR": taken.Addr().String(),
		"OLLAMETRY_ADMIN_ADDR":  freeAddr(t),
	}
	err = run(t.Context(), nil, func(k string) string { return env[k] }, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "listen for proxy") {
		t.Fatalf("run = %v, want a listen for proxy error", err)
	}
}

// safeBuffer is a bytes.Buffer safe for the logger's concurrent writes.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// healthcheck is the container's health probe (distroless has no curl): it
// must report only a live admin listener as healthy.
func TestHealthcheck(t *testing.T) {
	t.Parallel()
	serve := func(status int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/livez" {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(status)
		}))
		t.Cleanup(srv.Close)
		_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
		// The probe always dials loopback, whatever host the address names.
		return net.JoinHostPort("0.0.0.0", port)
	}
	tests := []struct {
		name, addr string
		want       int
	}{
		{"live", serve(http.StatusOK), 0},
		{"unhealthy status", serve(http.StatusServiceUnavailable), 1},
		{"nothing listening", freeAddr(t), 1},
		{"invalid address", "no-port", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := healthcheck(tt.addr); got != tt.want {
				t.Errorf("healthcheck(%q) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}
