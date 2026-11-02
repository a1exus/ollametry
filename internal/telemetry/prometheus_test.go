package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"

	"github.com/a1exus/ollametry/internal/genai"
)

// The Prometheus endpoint must expose the same series as the OTLP path:
// same names, labels, views and buckets, without scope labels.
func TestPrometheusReader(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	promReader, handler, err := NewPrometheusReader()
	if err != nil {
		t.Fatalf("NewPrometheusReader: %v", err)
	}
	mp, err := NewMeterProvider(ctx, ProviderConfig{
		ServiceVersion: "test", CardinalityLimit: 100,
		Reader: sdkmetric.NewManualReader(), ExtraReaders: []sdkmetric.Reader{promReader},
	})
	if err != nil {
		t.Fatalf("NewMeterProvider: %v", err)
	}
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	in, err := NewInstruments(mp.Meter(MeterName))
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}
	in.TokenUsage.Record(ctx, 21, metric.WithAttributes(
		genai.KeyRequestModel.String("granite4.2:8b"), genai.KeyTokenType.String(genai.TokenTypeInput)))
	in.HTTPDuration.Record(ctx, 0.5, httpconv.RequestMethodPost, "http", attribute.String("http.route", "/api/chat"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics = %d", rec.Code)
	}
	body := rec.Body.String()

	tests := []struct {
		name, want string
	}{
		{"token usage with its labels", `gen_ai_client_token_usage_sum{gen_ai_request_model="granite4.2:8b",gen_ai_token_type="input"} 21`},
		{"http duration uses the widened buckets", `http_server_request_duration_seconds_bucket{http_request_method="POST",http_route="/api/chat",url_scheme="http",le="655.36"} 1`},
		{"resource as target_info", `target_info{service_name="ollametry",service_version="test"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(body, tt.want) {
				t.Errorf("/metrics lacks %s", tt.want)
			}
		})
	}
	if strings.Contains(body, "otel_scope_") {
		t.Error("/metrics carries otel_scope_* labels, which the OTLP path does not")
	}
}
