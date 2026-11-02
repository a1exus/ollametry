package proxy

import (
	"log/slog"
	"net/url"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	"github.com/a1exus/ollametry/internal/telemetry"
)

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
		{`{"Model":"upper","prompt":"x`, "upper"},
		{`{"model":"first","model":"last","prompt":"x`, "last"},
		{`{"model":"a","model":null,"prompt":"x`, "a"},
		{`{"model":"a","model":7}`, ""},
		{`{"model":"cut mid-na`, ""},
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
