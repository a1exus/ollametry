package ollama

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// pollerHarness is a Poller with its gauges registered on an in-memory
// reader and a fixed clock.
type pollerHarness struct {
	poller *Poller
	reader *sdkmetric.ManualReader
}

func newPollerFor(t *testing.T, u *url.URL, client *http.Client) *pollerHarness {
	t.Helper()
	p := NewPoller(NewClient(u, client), 15*time.Second, slog.New(slog.DiscardHandler))
	// Five minutes before the /api/ps fixture's expires_at (21:23:55.400502-04:00).
	p.now = func() time.Time { return time.Date(2026, 11, 1, 1, 18, 55, 400502000, time.UTC) }

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if _, err := p.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
	return &pollerHarness{poller: p, reader: reader}
}

// newOfflinePoller never reaches a server: unit tests set its snapshot with
// store instead of polling.
func newOfflinePoller(t *testing.T) *pollerHarness {
	t.Helper()
	return newPollerFor(t, &url.URL{Scheme: "http", Host: "127.0.0.1:1"}, http.DefaultClient)
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

func TestPollerKeepAliveForeverOmitted(t *testing.T) {
	t.Parallel()
	h := newOfflinePoller(t)
	h.poller.store(Snapshot{
		At: h.poller.now(), Up: true, Version: "x",
		Running: []RunningModel{{Name: "m", Size: 1, ExpiresAt: time.Date(2318, 1, 1, 0, 0, 0, 0, time.UTC)}},
	})
	if g := h.gauges(t); len(g["ollama.model.keep_alive.remaining"]) != 0 {
		t.Errorf("keep_alive.remaining reported for a model kept loaded forever: %v", g["ollama.model.keep_alive.remaining"])
	}
}

func TestPollerKnownModel(t *testing.T) {
	t.Parallel()
	h := newOfflinePoller(t)
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
