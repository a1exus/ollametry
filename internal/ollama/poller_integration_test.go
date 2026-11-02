//go:build integration

package ollama

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// fakeOllama serves the recorded /api/version, /api/ps and /api/tags
// responses; failing makes every endpoint, or only /api/ps, fail.
type fakeOllama struct {
	t         *testing.T
	down      atomic.Bool
	psFailing atomic.Bool
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.down.Load() || (r.URL.Path == "/api/ps" && f.psFailing.Load()) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		return
	}
	files := map[string]string{"/api/version": "api_version.json", "/api/ps": "api_ps.json", "/api/tags": "api_tags.json"}
	name, ok := files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		f.t.Errorf("read fixture: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(b)
}

// liveHarness polls a fakeOllama over HTTP.
type liveHarness struct {
	*pollerHarness
	fake *fakeOllama
}

func newPollerHarness(t *testing.T) *liveHarness {
	t.Helper()
	fake := &fakeOllama{t: t}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fake Ollama URL: %v", err)
	}
	return &liveHarness{pollerHarness: newPollerFor(t, u, srv.Client()), fake: fake}
}

func TestPollerReportsRecordedState(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	h.poller.Poll(t.Context())
	g := h.gauges(t)

	if got := value(t, g["ollama.up"], "ollama.up"); got != 1 {
		t.Errorf("ollama.up = %d, want 1", got)
	}
	if n := g["ollama.up"][0].Attributes.Len(); n != 0 {
		t.Errorf("ollama.up has %d attributes, want none", n)
	}
	if got := value(t, g["ollama.build.info"], "ollama.build.info"); got != 1 {
		t.Errorf("ollama.build.info = %d, want 1", got)
	}
	if v, _ := g["ollama.build.info"][0].Attributes.Value("ollama.version"); v.AsString() != "0.40.13" {
		t.Errorf("ollama.version = %q, want 0.40.13", v.AsString())
	}
	tests := []struct {
		metric string
		want   int64
	}{
		{"ollama.model.memory.size", 5563429682},
		{"ollama.model.memory.vram.size", 5563429682},
		{"ollama.model.context_length", 4096},
		{"ollama.model.keep_alive.remaining", 300},
		{"ollama.models.installed", 10},
		{"ollama.models.storage.size", 121350174203},
	}
	for _, tt := range tests {
		t.Run(tt.metric, func(t *testing.T) {
			if got := value(t, g[tt.metric], tt.metric); got != tt.want {
				t.Errorf("%s = %d, want %d", tt.metric, got, tt.want)
			}
		})
	}

	attrs := g["ollama.model.memory.size"][0].Attributes
	for key, want := range map[string]string{
		"ollama.model.name":           "granite4.2:8b",
		"ollama.model.family":         "granite",
		"ollama.model.parameter_size": "8.8B",
		"ollama.model.quantization":   "Q4_K_M",
		"ollama.model.runner":         "llamacpp",
	} {
		if v, _ := attrs.Value(attribute.Key(key)); v.AsString() != want {
			t.Errorf("%s = %q, want %q", key, v.AsString(), want)
		}
	}
	if !h.poller.Ready() {
		t.Error("Ready() = false after a successful poll")
	}
}

func TestPollerStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		poll          bool
		down          bool
		psFailing     bool
		wantUp        []int64 // nil means no ollama.up point at all
		wantModels    bool
		wantInstalled bool
		wantReady     bool
	}{
		{name: "before first poll", poll: false, wantUp: nil},
		{name: "ollama down", poll: true, down: true, wantUp: []int64{0}},
		{name: "ps failing", poll: true, psFailing: true, wantUp: []int64{1}, wantInstalled: true, wantReady: true},
		{name: "healthy", poll: true, wantUp: []int64{1}, wantModels: true, wantInstalled: true, wantReady: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newPollerHarness(t)
			h.fake.down.Store(tt.down)
			h.fake.psFailing.Store(tt.psFailing)
			if tt.poll {
				h.poller.Poll(t.Context())
			}
			g := h.gauges(t)

			var up []int64
			for _, dp := range g["ollama.up"] {
				up = append(up, dp.Value)
			}
			if len(up) != len(tt.wantUp) || (len(up) == 1 && up[0] != tt.wantUp[0]) {
				t.Errorf("ollama.up = %v, want %v", up, tt.wantUp)
			}
			if got := len(g["ollama.model.memory.size"]) > 0; got != tt.wantModels {
				t.Errorf("loaded model gauges present = %v, want %v", got, tt.wantModels)
			}
			if got := len(g["ollama.models.installed"]) > 0; got != tt.wantInstalled {
				t.Errorf("installed gauge present = %v, want %v", got, tt.wantInstalled)
			}
			if got := h.poller.Ready(); got != tt.wantReady {
				t.Errorf("Ready() = %v, want %v", got, tt.wantReady)
			}
		})
	}
}

func TestPollerNotReadyWhenStale(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	h.poller.Poll(t.Context())
	polledAt := h.poller.Snapshot().At
	h.poller.now = func() time.Time { return polledAt.Add(time.Minute) }
	if h.poller.Ready() {
		t.Error("Ready() = true for a snapshot older than three intervals")
	}
}

// Up and down must be the same series, or the last "up" series lingers in
// Prometheus after Ollama stops (OTLP sends no staleness markers).
func TestPollerUpAndDownShareSeries(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	h.poller.Poll(t.Context())
	up := h.gauges(t)["ollama.up"]
	h.fake.down.Store(true)
	h.poller.Poll(t.Context())
	g := h.gauges(t)
	down := g["ollama.up"]

	if value(t, up, "ollama.up") != 1 || value(t, down, "ollama.up") != 0 {
		t.Fatalf("ollama.up went %v -> %v, want 1 -> 0", up, down)
	}
	if !up[0].Attributes.Equals(&down[0].Attributes) {
		t.Errorf("up and down use different attributes: %v vs %v", up[0].Attributes.ToSlice(), down[0].Attributes.ToSlice())
	}
	if len(g["ollama.build.info"]) != 0 {
		t.Errorf("ollama.build.info reported while down: %v", g["ollama.build.info"])
	}
}

func TestPollerCanceledPollKeepsLastState(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	h.poller.Poll(t.Context())
	want := h.poller.Snapshot()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h.poller.Poll(ctx)

	if got := h.poller.Snapshot(); !got.Up || got.Partial || !got.At.Equal(want.At) {
		t.Errorf("snapshot after a canceled poll = up %v partial %v at %v, want the previous one (up, at %v)",
			got.Up, got.Partial, got.At, want.At)
	}
}

// Run polls at once, keeps polling on its interval, and returns promptly
// when its context is canceled, which shutdown relies on.
func TestPollerRunPollsUntilCanceled(t *testing.T) {
	t.Parallel()
	var polls atomic.Int32
	fake := &fakeOllama{t: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			polls.Add(1)
		}
		fake.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse fake Ollama URL: %v", err)
	}
	p := NewPoller(NewClient(u, srv.Client()), 20*time.Millisecond, slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.Run(ctx)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for polls.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("%d polls in 5s at a 20ms interval, want at least 3", polls.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !p.Ready() {
		t.Error("Ready() = false after successful polls")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s of cancellation")
	}
}
