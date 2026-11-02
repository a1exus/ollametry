# Metrics

[← README](../README.md)

- [What counts as inference](#what-counts-as-inference)
- [Metric reference](#metric-reference)
- [Attributes](#attributes)
- [error.type](#errortype)
- [Limits](#limits)
- [Querying in Prometheus](#querying-in-prometheus)

Metrics follow the OpenTelemetry
[GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
where they exist; Ollama-specific ones are under `ollama.*`.
They reach Prometheus either by OTLP push through otel-lgtm or by scraping
`/metrics` ([Prometheus exporter](prometheus.md)); names and labels are the
same both ways.

## What counts as inference

Requests to `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`,
`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/messages`
(Anthropic) and `/v1/responses` are inference requests. Everything else
(pulls, `/api/tags`, blob uploads) is relayed and counted in `http.server.*`
only.

## Metric reference

| Metric | Type | What |
|---|---|---|
| `gen_ai.client.operation.duration` | histogram, s | Whole inference request at the proxy; `error.type` on failures |
| `gen_ai.client.token.usage` | histogram, `{token}` | Input and output tokens per request (`gen_ai.token.type`); embeddings have input only |
| `gen_ai.server.time_to_first_token` | histogram, s | Streamed responses: request to first generated token, reasoning included |
| `gen_ai.server.time_per_output_token` | histogram, s | Decode time per token, from Ollama's own timing when available |
| `ollama.prompt.cached_tokens` | counter, `{token}` | Input tokens served from the prompt cache |
| `ollama.model.load.duration` | histogram, s | Model load time as Ollama reports it (cold starts) |
| `ollama.prompt.eval.duration` | histogram, s | Prompt evaluation time as Ollama reports it |
| `ollama.inference.active` | up-down counter | Inference requests in flight |
| `http.server.request.duration` | histogram, s | Every proxied request, by `http.route` and status |
| `http.server.active_requests` | up-down counter | Every proxied request in flight |
| `ollama.up` | gauge | 1 if the last poll reached Ollama, else 0 |
| `ollama.build.info` | gauge | 1 while Ollama is reachable, with `ollama.version` |
| `ollama.model.memory.size`, `ollama.model.memory.vram.size` | gauge, By | Memory per loaded model |
| `ollama.model.context_length` | gauge, `{token}` | Context a loaded model runs with |
| `ollama.model.keep_alive.remaining` | gauge, s | Time until an idle model is unloaded; absent when kept forever |
| `ollama.models.installed`, `ollama.models.storage.size` | gauge | Local models and their disk use |

Go runtime metrics for the proxy itself are exported too.

## Attributes

Shared attributes on `gen_ai.*`: `gen_ai.operation.name` (`chat`,
`text_completion`, `embeddings`), `gen_ai.provider.name=ollama`,
`gen_ai.request.model`, `gen_ai.response.model`, `http.route`,
`server.address`, `server.port`.

`gen_ai.request.model` is `_OTHER` for a name that is not installed or loaded
(checked against the last `/api/tags` and `/api/ps` poll, as Ollama resolves
names: case-insensitive, `:latest` by default) or longer than 256 bytes, so
typos and probes cannot fill the cardinality limit. The log line keeps the
name as sent.

Per-model gauges carry `ollama.model.name`, `ollama.model.family`,
`ollama.model.parameter_size`, `ollama.model.quantization` and
`ollama.model.runner`.

## error.type

- An HTTP status code (`404`, `500`, ...).
- The API's own error type for errors reported inside a 200 response
  (`stream_error` if it has none).
- `incomplete`: a stream ended without its final record.
- `canceled`: the client hung up; Ollama is told to stop generating.
- `upstream_error`: Ollama unreachable or the connection dropped.

On `http.server.request.duration`, 4xx responses carry no `error.type`: they
are the client's error, not the server's.

## Limits

- OpenAI-compatible streams carry token counts only when the client sends
  `"stream_options": {"include_usage": true}`. Without it, those requests have
  duration and time to first token but no token usage.
- Time to first token exists only for streamed responses.
- Request bodies and response records larger than
  `OLLAMETRY_MAX_INSPECT_BYTES` are relayed but only partly measured.

## Querying in Prometheus

OTLP names become Prometheus names with `.` → `_`, a unit suffix (`_seconds`,
`_bytes`) and `_total` on counters: `gen_ai.client.token.usage` is
`gen_ai_client_token_usage_bucket`/`_sum`/`_count`.

Two OTLP behaviours to keep in mind when writing queries:

- **A counter's first export is its baseline.** A model's very first request
  does not show up in `rate()` or `increase()`.
- **Series linger 5 minutes after they stop.** OTLP sends no staleness
  markers, so a gauge keeps its last value after a model unloads, and series
  from a replaced container stay with the old `service_version`. Aggregate by
  model name rather than counting raw series, and filter gauges by age:

  ```promql
  ollama_model_memory_size_bytes
    and (time() - timestamp(ollama_model_memory_size_bytes) < 45)
  ```

The dashboard in [`deploy/grafana/ollametry.json`](../deploy/grafana/ollametry.json)
follows both rules.
