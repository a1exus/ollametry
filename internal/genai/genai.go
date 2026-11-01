// Package genai defines the OpenTelemetry GenAI semantic convention names the
// proxy emits.
//
// The OpenTelemetry Go module does not generate these yet because the GenAI
// conventions are in Development status upstream, so they are kept here, in
// one place, against https://opentelemetry.io/docs/specs/semconv/gen-ai/.
package genai

import "go.opentelemetry.io/otel/attribute"

// Attribute keys.
const (
	KeyOperationName = attribute.Key("gen_ai.operation.name")
	KeyProviderName  = attribute.Key("gen_ai.provider.name")
	KeyRequestModel  = attribute.Key("gen_ai.request.model")
	KeyResponseModel = attribute.Key("gen_ai.response.model")
	KeyTokenType     = attribute.Key("gen_ai.token.type")
)

// Well-known attribute values.
const (
	ProviderOllama = "ollama"

	OperationChat           = "chat"
	OperationTextCompletion = "text_completion"
	OperationEmbeddings     = "embeddings"

	TokenTypeInput  = "input"
	TokenTypeOutput = "output"
)

// Metric names, units and descriptions.
const (
	MetricTokenUsage         = "gen_ai.client.token.usage"
	MetricOperationDuration  = "gen_ai.client.operation.duration"
	MetricTimeToFirstToken   = "gen_ai.server.time_to_first_token"
	MetricTimePerOutputToken = "gen_ai.server.time_per_output_token"

	UnitToken   = "{token}"
	UnitSeconds = "s"
)

// Bucket boundaries.
//
// They start from the spec's advice and extend the upper end where a local
// model on consumer hardware routinely exceeds it: cold model loads push
// time to first token past 10 s, long generations run for minutes, and small
// models on Apple Silicon decode faster than 10 ms per token.
var (
	TokenUsageBuckets = []float64{
		1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864,
	}
	OperationDurationBuckets = []float64{
		0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92,
		163.84, 327.68, 655.36,
	}
	TimeToFirstTokenBuckets = []float64{
		0.001, 0.005, 0.01, 0.02, 0.04, 0.06, 0.08, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10,
		20, 40, 80,
	}
	TimePerOutputTokenBuckets = []float64{
		0.0025, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.75, 1, 2.5,
	}
)
