package telemetry

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// NewPrometheusReader returns a reader for the meter provider and the
// /metrics handler that serves what it collects, so Prometheus can scrape
// ollametry directly. It sees the same instruments, views and cardinality
// limit as the OTLP exporter, and drops the otel_scope_* labels so series
// match the ones the OTLP path produces in Prometheus. Its registry is
// private: Go runtime metrics already come from the OTel runtime
// instrumentation.
func NewPrometheusReader() (sdkmetric.Reader, http.Handler, error) {
	reg := prometheus.NewRegistry()
	reader, err := otelprom.New(otelprom.WithRegisterer(reg), otelprom.WithoutScopeInfo())
	if err != nil {
		return nil, nil, fmt.Errorf("create Prometheus exporter: %w", err)
	}
	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
	return reader, handler, nil
}
