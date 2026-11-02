package proxy

import (
	"log/slog"
	"net/url"
	"strings"
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

func TestLogValuesAreBounded(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 10_000)
	tests := []struct {
		name, got, want string
	}{
		{"known method kept", logMethod("POST"), "POST"},
		{"unknown method folded", logMethod("PROPFIND"), "_OTHER"},
		{"long method folded", logMethod(strings.Repeat("X", 10_000)), "_OTHER"},
		{"short path kept", logPath("/api/chat"), "/api/chat"},
		{"long path cut", logPath("/" + long), "/" + long[:maxLoggedPathLen-1] + "…"},
		{"cut never splits a rune", logPath(strings.Repeat("é", maxLoggedPathLen)), strings.Repeat("é", maxLoggedPathLen/2) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.got != tt.want {
				t.Errorf("got %q (%d bytes), want %q (%d bytes)", tt.got, len(tt.got), tt.want, len(tt.want))
			}
		})
	}
}

// Model names over maxModelNameLen are not model names and would only add
// series, so they fold into _OTHER; the limit itself is kept.
func TestSanitizeModel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, model, want string
	}{
		{"empty", "", ""},
		{"ordinary", "granite4.2:8b", "granite4.2:8b"},
		{"at the limit", strings.Repeat("m", maxModelNameLen), strings.Repeat("m", maxModelNameLen)},
		{"over the limit", strings.Repeat("m", maxModelNameLen+1), otherModel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeModel(tt.model); got != tt.want {
				t.Errorf("sanitizeModel(%d bytes) = %.20q…, want %.20q…", len(tt.model), got, tt.want)
			}
		})
	}
}
