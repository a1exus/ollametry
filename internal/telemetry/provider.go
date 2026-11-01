// Package telemetry builds the OpenTelemetry meter provider and the
// instruments the proxy records into.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/a1exus/ollametry/internal/genai"
)

// ServiceName is the default service.name; OTEL_SERVICE_NAME overrides it.
const ServiceName = "ollametry"

// ProviderConfig configures NewMeterProvider.
type ProviderConfig struct {
	ServiceVersion   string
	CardinalityLimit int
	// Reader overrides the environment-configured reader; used by tests.
	Reader sdkmetric.Reader
}

// NewMeterProvider builds a meter provider whose exporter is configured by
// the standard OTEL_* environment variables (OTEL_METRICS_EXPORTER,
// OTEL_EXPORTER_OTLP_*, OTEL_METRIC_EXPORT_INTERVAL, OTEL_RESOURCE_ATTRIBUTES,
// OTEL_SERVICE_NAME). It also starts Go runtime metrics for the proxy itself.
func NewMeterProvider(ctx context.Context, cfg ProviderConfig) (*sdkmetric.MeterProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", ServiceName),
			attribute.String("service.version", cfg.ServiceVersion),
		),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	reader := cfg.Reader
	if reader == nil {
		if reader, err = autoexport.NewMetricReader(ctx); err != nil {
			return nil, fmt.Errorf("create metric reader from OTEL_* environment: %w", err)
		}
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
		sdkmetric.WithCardinalityLimit(cfg.CardinalityLimit),
		sdkmetric.WithView(httpDurationView()),
	)
	if err := runtime.Start(runtime.WithMeterProvider(mp), runtime.WithMinimumReadMemStatsInterval(15*time.Second)); err != nil {
		return nil, fmt.Errorf("start Go runtime metrics: %w", err)
	}
	return mp, nil
}

// httpDurationView widens http.server.request.duration buckets: the semconv
// default stops at 10 s, while proxied generations routinely run for minutes.
func httpDurationView() sdkmetric.View {
	return sdkmetric.NewView(
		sdkmetric.Instrument{Name: "http.server.request.duration"},
		sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
			Boundaries: genai.OperationDurationBuckets,
		}},
	)
}
