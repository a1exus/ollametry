package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/a1exus/ollametry/internal/telemetry"
)

// replayTransport answers every request with a recorded response from
// memory, so benchmarks measure the proxy, not a network.
type replayTransport struct {
	body        []byte
	contentType string
}

func (rt replayTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {rt.contentType}},
		Body:          io.NopCloser(bytes.NewReader(rt.body)),
		ContentLength: -1,
		Request:       r,
	}, nil
}

var benchCases = []struct {
	name, path, file, contentType string
}{
	{"native chat stream", "/api/chat", "native_chat_stream.ndjson", "application/x-ndjson"},
	{"native chat", "/api/chat", "native_chat.json", "application/json; charset=utf-8"},
	{"openai chat stream", "/v1/chat/completions", "openai_chat_stream_usage.sse", "text/event-stream"},
	{"anthropic stream", "/v1/messages", "anthropic_messages_stream.sse", "text/event-stream"},
}

const benchRequest = `{"model":"granite4.2:8b","messages":[{"role":"user","content":"Say hi in three words."}],"stream":true}`

func benchProxy(tb testing.TB, rt http.RoundTripper) *Proxy {
	tb.Helper()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
	in, err := telemetry.NewInstruments(mp.Meter("bench"))
	if err != nil {
		tb.Fatalf("NewInstruments: %v", err)
	}
	p, err := New(Config{
		Upstream:        &url.URL{Scheme: "http", Host: "127.0.0.1:11434"},
		MaxInspectBytes: 32 << 20,
		Instruments:     in,
		Logger:          slog.New(slog.DiscardHandler),
		Transport:       rt,
	})
	if err != nil {
		tb.Fatalf("New: %v", err)
	}
	return p
}

// BenchmarkProxy measures one request through the whole proxy: request
// inspection, relaying, framing, decoding and recording every metric.
// BenchmarkBaseline does the same relay without ollametry; the difference is
// the proxy's overhead.
func BenchmarkProxy(b *testing.B) {
	for _, c := range benchCases {
		body := readFixtureForBench(b, c.file)
		p := benchProxy(b, replayTransport{body: body, contentType: c.contentType})
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(benchRequest))
				rec := httptest.NewRecorder()
				p.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK || rec.Body.Len() != len(body) {
					b.Fatalf("status %d, %d bytes relayed, want 200 and %d", rec.Code, rec.Body.Len(), len(body))
				}
			}
		})
	}
}

func BenchmarkBaseline(b *testing.B) {
	for _, c := range benchCases {
		body := readFixtureForBench(b, c.file)
		rt := replayTransport{body: body, contentType: c.contentType}
		b.Run(c.name, func(b *testing.B) {
			b.SetBytes(int64(len(body)))
			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequest(http.MethodPost, c.path, strings.NewReader(benchRequest))
				rec := httptest.NewRecorder()
				resp, err := rt.RoundTrip(req)
				if err != nil {
					b.Fatal(err)
				}
				rec.WriteHeader(resp.StatusCode)
				_, _ = io.Copy(rec, resp.Body)
				_ = resp.Body.Close()
			}
		})
	}
}

func readFixtureForBench(tb testing.TB, name string) []byte {
	tb.Helper()
	body, err := os.ReadFile(filepath.Join("..", "dialect", "testdata", name))
	if err != nil {
		tb.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// syntheticStream is a native chat stream of n token records and a final
// record, built from the recorded fixture.
func syntheticStream(tb testing.TB, n int) []byte {
	tb.Helper()
	lines := bytes.SplitAfter(readFixtureForBench(tb, "native_chat_stream.ndjson"), []byte("\n"))
	token, final := lines[1], lines[len(lines)-2]
	var buf bytes.Buffer
	for range n {
		buf.Write(token)
	}
	buf.Write(final)
	return buf.Bytes()
}

func allocsForStream(t *testing.T, n int) float64 {
	t.Helper()
	p := benchProxy(t, replayTransport{body: syntheticStream(t, n), contentType: "application/x-ndjson"})
	return testing.AllocsPerRun(20, func() {
		p.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(benchRequest)))
	})
}

// maxAllocsPerRecord bounds the allocations each streamed record adds (2.0
// today; 3.8 under -race). A long generation relays thousands of records, so
// per-record cost is what scales; the fixed per-request cost shows in the
// benchmarks. One extra allocation per record fails either ceiling.
func maxAllocsPerRecord() float64 {
	if raceEnabled {
		return 4.5
	}
	return 2.5
}

func TestProxyAllocationsPerRecord(t *testing.T) {
	const short, long = 100, 1100
	perRecord := (allocsForStream(t, long) - allocsForStream(t, short)) / (long - short)
	t.Logf("%.2f allocations per streamed record", perRecord)
	if limit := maxAllocsPerRecord(); perRecord > limit {
		t.Errorf("%.2f allocations per streamed record, want at most %.1f", perRecord, limit)
	}
}
