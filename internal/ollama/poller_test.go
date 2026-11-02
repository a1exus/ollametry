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
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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

type pollerHarness struct {
	fake   *fakeOllama
	poller *Poller
	reader *sdkmetric.ManualReader
}

func newPollerHarness(t *testing.T) *pollerHarness {
	t.Helper()
	fake := &fakeOllama{t: t}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)

	p := NewPoller(NewClient(u, srv.Client()), 15*time.Second, slog.New(slog.DiscardHandler))
	// Five minutes before the /api/ps fixture's expires_at (21:23:55.400502-04:00).
	p.now = func() time.Time { return time.Date(2026, 11, 1, 1, 18, 55, 400502000, time.UTC) }

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if _, err := p.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
	return &pollerHarness{fake: fake, poller: p, reader: reader}
}

// gauges returns every int64 gauge point by metric name.
func (h *pollerHarness) gauges(t *testing.T) map[string][]metricdata.DataPoint[int64] {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	out := map[string][]metricdata.DataPoint[int64]{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok {
				out[m.Name] = g.DataPoints
			}
		}
	}
	return out
}

func value(t *testing.T, points []metricdata.DataPoint[int64], name string) int64 {
	t.Helper()
	if len(points) != 1 {
		t.Fatalf("%s: %d data points, want 1", name, len(points))
	}
	return points[0].Value
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

func TestPollerKeepAliveForeverOmitted(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	h.poller.store(Snapshot{
		At: h.poller.now(), Up: true, Version: "x",
		Running: []RunningModel{{Name: "m", Size: 1, ExpiresAt: time.Date(2318, 1, 1, 0, 0, 0, 0, time.UTC)}},
	})
	if g := h.gauges(t); len(g["ollama.model.keep_alive.remaining"]) != 0 {
		t.Errorf("keep_alive.remaining reported for a model kept loaded forever: %v", g["ollama.model.keep_alive.remaining"])
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

func TestPollerKnownModel(t *testing.T) {
	t.Parallel()
	h := newPollerHarness(t)
	if !h.poller.KnownModel("anything") {
		t.Error("KnownModel = false before the first poll, want true (inventory unknown)")
	}
	h.poller.store(Snapshot{
		At: h.poller.now(), Up: true,
		Installed: []InstalledModel{{Name: "granite4.2:8b"}, {Name: "llama3:latest"}, {Name: "hf.co/org/repo:Q4_K_M"}},
		Running:   []RunningModel{{Name: "loaded-only:1b"}},
	})
	tests := []struct {
		name string
		want bool
	}{
		{"granite4.2:8b", true},
		{"Granite4.2:8B", true},
		{"llama3", true},
		{"llama3:latest", true},
		{"hf.co/org/repo:q4_k_m", true},
		{"loaded-only:1b", true},
		{"granite4.2", false},
		{"granite4.2:7b", false},
		{"typo-model", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := h.poller.KnownModel(tt.name); got != tt.want {
				t.Errorf("KnownModel(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}
