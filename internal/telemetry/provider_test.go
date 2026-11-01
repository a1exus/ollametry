package telemetry

import (
	"context"
	"slices"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"

	"github.com/a1exus/ollametry/internal/genai"
)

const runtimeScope = "go.opentelemetry.io/contrib/instrumentation/runtime"

// newTestProvider builds the real provider on a ManualReader, records one
// value into every histogram and returns the collected metrics.
func newTestProvider(t *testing.T, cfg ProviderConfig) metricdata.ResourceMetrics {
	t.Helper()
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	cfg.Reader = reader
	mp, err := NewMeterProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("NewMeterProvider: %v", err)
	}
	t.Cleanup(func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown meter provider: %v", err)
		}
	})

	in, err := NewInstruments(mp.Meter(MeterName))
	if err != nil {
		t.Fatalf("NewInstruments: %v", err)
	}
	in.HTTPDuration.Record(ctx, 0.5, httpconv.RequestMethodPost, "http")
	in.TokenUsage.Record(ctx, 10)
	in.OperationDuration.Record(ctx, 1)
	in.TimeToFirstToken.Record(ctx, 0.1)
	in.TimePerOutputToken.Record(ctx, 0.01)
	in.ModelLoadDuration.Record(ctx, 2)
	in.PromptEvalDuration.Record(ctx, 0.2)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	return rm
}

func findMetric(rm metricdata.ResourceMetrics, scope, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		if sm.Scope.Name != scope {
			continue
		}
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestHistogramBuckets(t *testing.T) {
	t.Parallel()
	rm := newTestProvider(t, ProviderConfig{ServiceVersion: "test", CardinalityLimit: 100})

	tests := []struct {
		name string
		want []float64
	}{
		// The semconv default stops at 10 s; the view must win over httpconv's
		// own advisory buckets.
		{"http.server.request.duration", genai.OperationDurationBuckets},
		{genai.MetricOperationDuration, genai.OperationDurationBuckets},
		{genai.MetricTimeToFirstToken, genai.TimeToFirstTokenBuckets},
		{genai.MetricTimePerOutputToken, genai.TimePerOutputTokenBuckets},
		{"ollama.model.load.duration", loadDurationBuckets},
		{"ollama.prompt.eval.duration", genai.OperationDurationBuckets},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, ok := findMetric(rm, MeterName, tt.name)
			if !ok {
				t.Fatalf("metric %s not collected", tt.name)
			}
			h, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("metric %s is %T, want a float64 histogram", tt.name, m.Data)
			}
			if got := h.DataPoints[0].Bounds; !slices.Equal(got, tt.want) {
				t.Errorf("bounds = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run(genai.MetricTokenUsage, func(t *testing.T) {
		t.Parallel()
		m, ok := findMetric(rm, MeterName, genai.MetricTokenUsage)
		if !ok {
			t.Fatalf("metric %s not collected", genai.MetricTokenUsage)
		}
		h, ok := m.Data.(metricdata.Histogram[int64])
		if !ok {
			t.Fatalf("metric is %T, want an int64 histogram", m.Data)
		}
		if got := h.DataPoints[0].Bounds; !slices.Equal(got, genai.TokenUsageBuckets) {
			t.Errorf("bounds = %v, want %v", got, genai.TokenUsageBuckets)
		}
	})
}

func TestResource(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		want        string
	}{
		{"default service name", "", ServiceName},
		{"OTEL_SERVICE_NAME wins", "ollametry-dev", "ollametry-dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_SERVICE_NAME", tt.serviceName)
			rm := newTestProvider(t, ProviderConfig{ServiceVersion: "1.2.3", CardinalityLimit: 100})

			for key, want := range map[attribute.Key]string{
				"service.name":    tt.want,
				"service.version": "1.2.3",
			} {
				if got, _ := rm.Resource.Set().Value(key); got.AsString() != want {
					t.Errorf("%s = %q, want %q", key, got.AsString(), want)
				}
			}
		})
	}
}

func TestCardinalityLimit(t *testing.T) {
	t.Parallel()
	const limit = 10
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp, err := NewMeterProvider(ctx, ProviderConfig{ServiceVersion: "test", CardinalityLimit: limit, Reader: reader})
	if err != nil {
		t.Fatalf("NewMeterProvider: %v", err)
	}
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	counter, err := mp.Meter(MeterName).Int64Counter("test.series")
	if err != nil {
		t.Fatalf("create counter: %v", err)
	}
	for i := range limit * 3 {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.Int("model", i)))
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	m, ok := findMetric(rm, MeterName, "test.series")
	if !ok {
		t.Fatal("test.series not collected")
	}
	points := m.Data.(metricdata.Sum[int64]).DataPoints
	if len(points) != limit {
		t.Fatalf("got %d series, want the limit %d", len(points), limit)
	}
	var total int64
	overflow := false
	for _, p := range points {
		total += p.Value
		if v, ok := p.Attributes.Value("otel.metric.overflow"); ok && v.AsBool() {
			overflow = true
		}
	}
	if !overflow || total != limit*3 {
		t.Errorf("overflow series present = %v, total = %d; want true and %d (no measurement lost)", overflow, total, limit*3)
	}
}

func TestRuntimeMetrics(t *testing.T) {
	t.Parallel()
	rm := newTestProvider(t, ProviderConfig{ServiceVersion: "test", CardinalityLimit: 100})
	if _, ok := findMetric(rm, runtimeScope, "go.goroutine.count"); !ok {
		t.Errorf("Go runtime metrics not collected from scope %s", runtimeScope)
	}
}
