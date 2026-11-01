package telemetry

import (
	"errors"
	"fmt"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"

	"github.com/a1exus/ollametry/internal/genai"
)

// MeterName is the instrumentation scope of every instrument the proxy owns.
const MeterName = "ollametry"

// Instruments are the synchronous instruments recorded per proxied request.
type Instruments struct {
	HTTPDuration httpconv.ServerRequestDuration
	HTTPActive   httpconv.ServerActiveRequests

	TokenUsage         metric.Int64Histogram
	OperationDuration  metric.Float64Histogram
	TimeToFirstToken   metric.Float64Histogram
	TimePerOutputToken metric.Float64Histogram

	CachedInputTokens  metric.Int64Counter
	ModelLoadDuration  metric.Float64Histogram
	PromptEvalDuration metric.Float64Histogram
	InferenceActive    metric.Int64UpDownCounter
}

// loadDurationBuckets cover a warm model (milliseconds) up to a cold load of
// a large model from disk.
var loadDurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 40, 80, 160}

// NewInstruments creates all instruments on meter.
func NewInstruments(meter metric.Meter) (*Instruments, error) {
	var (
		in   Instruments
		errs []error
	)
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}
	var err error

	in.HTTPDuration, err = httpconv.NewServerRequestDuration(meter)
	add(err)
	in.HTTPActive, err = httpconv.NewServerActiveRequests(meter)
	add(err)

	in.TokenUsage, err = meter.Int64Histogram(genai.MetricTokenUsage,
		metric.WithDescription("Number of input and output tokens used."),
		metric.WithUnit(genai.UnitToken),
		metric.WithExplicitBucketBoundaries(genai.TokenUsageBuckets...))
	add(err)
	in.OperationDuration, err = meter.Float64Histogram(genai.MetricOperationDuration,
		metric.WithDescription("GenAI operation duration, observed at the proxy."),
		metric.WithUnit(genai.UnitSeconds),
		metric.WithExplicitBucketBoundaries(genai.OperationDurationBuckets...))
	add(err)
	in.TimeToFirstToken, err = meter.Float64Histogram(genai.MetricTimeToFirstToken,
		metric.WithDescription("Time from request to the first generated token of a streamed response, reasoning included."),
		metric.WithUnit(genai.UnitSeconds),
		metric.WithExplicitBucketBoundaries(genai.TimeToFirstTokenBuckets...))
	add(err)
	in.TimePerOutputToken, err = meter.Float64Histogram(genai.MetricTimePerOutputToken,
		metric.WithDescription("Time per output token generated after the first token."),
		metric.WithUnit(genai.UnitSeconds),
		metric.WithExplicitBucketBoundaries(genai.TimePerOutputTokenBuckets...))
	add(err)

	in.CachedInputTokens, err = meter.Int64Counter("ollama.prompt.cached_tokens",
		metric.WithDescription("Input tokens served from the prompt cache instead of being evaluated."),
		metric.WithUnit(genai.UnitToken))
	add(err)
	in.ModelLoadDuration, err = meter.Float64Histogram("ollama.model.load.duration",
		metric.WithDescription("Time Ollama spent loading the model for a request, as it reports it."),
		metric.WithUnit(genai.UnitSeconds),
		metric.WithExplicitBucketBoundaries(loadDurationBuckets...))
	add(err)
	in.PromptEvalDuration, err = meter.Float64Histogram("ollama.prompt.eval.duration",
		metric.WithDescription("Time Ollama spent evaluating the prompt, as it reports it."),
		metric.WithUnit(genai.UnitSeconds),
		metric.WithExplicitBucketBoundaries(genai.OperationDurationBuckets...))
	add(err)
	in.InferenceActive, err = meter.Int64UpDownCounter("ollama.inference.active",
		metric.WithDescription("Inference requests in flight through the proxy."),
		metric.WithUnit("{request}"))
	add(err)

	if len(errs) > 0 {
		return nil, fmt.Errorf("create instruments: %w", errors.Join(errs...))
	}
	return &in, nil
}
