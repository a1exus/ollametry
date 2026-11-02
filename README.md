# ollametry

[![CI](https://github.com/a1exus/ollametry/actions/workflows/ci.yml/badge.svg)](https://github.com/a1exus/ollametry/actions/workflows/ci.yml)
[![CodeQL](https://github.com/a1exus/ollametry/actions/workflows/codeql.yml/badge.svg)](https://github.com/a1exus/ollametry/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/a1exus/ollametry/badge)](https://scorecard.dev/viewer/?uri=github.com/a1exus/ollametry)
[![Go](https://img.shields.io/github/go-mod/go-version/a1exus/ollametry)](go.mod)
[![License: MIT](https://img.shields.io/github/license/a1exus/ollametry)](LICENSE)

A transparent reverse proxy for [Ollama](https://ollama.com) that exports
OpenTelemetry metrics: token usage, latency, time to first token and decode
speed per model, error rates, and the models Ollama holds in memory. Metrics
follow the OpenTelemetry [GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
and go over OTLP to [`grafana/otel-lgtm`](https://github.com/grafana/docker-otel-lgtm)
(`~/opt/grafana`), with a ready-made Grafana dashboard.

Ollama has no metrics endpoint, so ollametry sits in front of it and reads the
usage statistics Ollama already returns in every response. Requests and
responses pass through byte for byte, streaming included.

```
 clients ──► ollametry :11435 ──► Ollama :11434 (macOS host)
                 │
                 ├─ per request: gen_ai.* and http.server.* metrics
                 └─ every 15 s:  /api/ps, /api/tags → ollama.* gauges
                 │
                 ▼ OTLP/HTTP
            otel-lgtm :4318 ──► Prometheus ──► Grafana dashboard
```

Docs: [`AGENTS.md`](AGENTS.md) (rules for AI agents),
[`CLAUDE.md`](CLAUDE.md) (architecture and gotchas),
[`CONTRIBUTING.md`](CONTRIBUTING.md), [`SECURITY.md`](SECURITY.md),
[`CHANGELOG.md`](CHANGELOG.md).

## Prerequisites

- Ollama running on the host (`ollama serve` or the macOS app) on `127.0.0.1:11434`.
- The `~/opt/grafana` stack running (`make up` there); ollametry joins its
  `grafana_default` network.
- Docker Compose v2.24 or newer. Host tools: `docker`, `make`, `curl`, `bash`.

## Quick start

```sh
make up                               # build, start, wait until healthy
make health                           # Ollama, the proxy, readiness
make smoke MODEL=granite4.2:8b        # one request through the proxy
make -C ~/opt/grafana dashboards     # provision the dashboard into Grafana
```

| Endpoint | URL |
|---|---|
| Proxy (use instead of Ollama) | `http://127.0.0.1:11435` |
| Liveness / readiness | `http://127.0.0.1:11436/livez`, `/readyz` |
| Dashboard (folder "ollama") | `https://grafana.localhost/d/ollametry` |

## Pointing clients at the proxy

**Only requests that go through the proxy are measured.** Anything that calls
`:11434` directly is invisible to the request metrics (the `/api/ps` gauges
still see every loaded model).

```sh
# Ollama CLI
OLLAMA_HOST=127.0.0.1:11435 ollama run granite4.2:8b

# OpenAI SDKs
export OPENAI_BASE_URL=http://127.0.0.1:11435/v1

# Anthropic SDKs and tools that speak the Messages API
export ANTHROPIC_BASE_URL=http://127.0.0.1:11435
```

To measure every client without reconfiguring them, move Ollama to another
port and put the proxy on 11434. This changes your Ollama setup, so it is not
done for you:

```sh
launchctl setenv OLLAMA_HOST 127.0.0.1:11433   # then restart the Ollama app
PROXY_PORT=11434 OLLAMA_UPSTREAM=http://host.docker.internal:11433 make up
```

## What is measured

Requests to `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`,
`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/messages`
(Anthropic) and `/v1/responses` are inference requests. Everything else
(pulls, `/api/tags`, blob uploads) is relayed and counted in `http.server.*`
only.

| Metric | Type | What |
|---|---|---|
| `gen_ai.client.operation.duration` | histogram, s | Whole inference request at the proxy; `error.type` on failures |
| `gen_ai.client.token.usage` | histogram, `{token}` | Input and output tokens per request (`gen_ai.token.type`) |
| `gen_ai.server.time_to_first_token` | histogram, s | Streamed responses: request to first generated token, reasoning included |
| `gen_ai.server.time_per_output_token` | histogram, s | Decode time per token, from Ollama's own timing when available |
| `ollama.prompt.cached_tokens` | counter, `{token}` | Input tokens served from the prompt cache |
| `ollama.model.load.duration` | histogram, s | Model load time as Ollama reports it (cold starts) |
| `ollama.prompt.eval.duration` | histogram, s | Prompt evaluation time as Ollama reports it |
| `ollama.inference.active` | up-down counter | Inference requests in flight |
| `http.server.request.duration` | histogram, s | Every proxied request, by `http.route` and status |
| `http.server.active_requests` | up-down counter | Every proxied request in flight |
| `ollama.up` | gauge | 1 if the last poll reached Ollama, with `ollama.version` |
| `ollama.model.memory.size`, `ollama.model.memory.vram.size` | gauge, By | Memory per loaded model |
| `ollama.model.context_length` | gauge, `{token}` | Context a loaded model runs with |
| `ollama.model.keep_alive.remaining` | gauge, s | Time until an idle model is unloaded; absent when kept forever |
| `ollama.models.installed`, `ollama.models.storage.size` | gauge | Local models and their disk use |

Shared attributes on `gen_ai.*`: `gen_ai.operation.name` (`chat`,
`text_completion`, `embeddings`), `gen_ai.provider.name=ollama`,
`gen_ai.request.model`, `gen_ai.response.model`, `http.route`,
`server.address`, `server.port`. Go runtime metrics for the proxy itself are
exported too.

`error.type` values: an HTTP status code (`404`, `500`, ...), the API's own
error type for errors reported inside a 200 response (`stream_error` if it has
none), `incomplete` (a stream ended without its final record), `canceled` (the
client hung up; Ollama is told to stop generating) and `upstream_error`
(Ollama unreachable or the connection dropped).

### Limits

- OpenAI-compatible streams carry token counts only when the client sends
  `"stream_options": {"include_usage": true}`. Without it, those requests have
  duration and time to first token but no token usage.
- Time to first token exists only for streamed responses.
- Prometheus sees each counter first at its current value, so a model's very
  first request does not show up in `rate()` or `increase()`.

## Configuration

Flags take precedence over `OLLAMETRY_*` variables; Compose sets the variables.

| Variable | Default (binary) | Compose default | Purpose |
|---|---|---|---|
| `OLLAMETRY_LISTEN_ADDR` | `127.0.0.1:11435` | `0.0.0.0:11435` (image) | Proxy listener |
| `OLLAMETRY_ADMIN_ADDR` | `127.0.0.1:11436` | `0.0.0.0:11436` (image) | `/livez`, `/readyz` |
| `OLLAMETRY_OLLAMA_URL` | `http://127.0.0.1:11434` | `http://host.docker.internal:11434` | Upstream Ollama |
| `OLLAMETRY_POLL_INTERVAL` | `15s` | `15s` | `/api/ps` and `/api/tags` polling |
| `OLLAMETRY_SHUTDOWN_TIMEOUT` | `30s` | `30s` | Grace period for in-flight streams |
| `OLLAMETRY_MAX_INSPECT_BYTES` | `33554432` | same | Largest request body / response record decoded; larger ones pass through unmeasured |
| `OLLAMETRY_CARDINALITY_LIMIT` | `2000` | same | Attribute sets per metric |
| `OLLAMETRY_LOG_LEVEL`, `OLLAMETRY_LOG_FORMAT` | `info`, `json` | same | Logging (`text` for humans) |

The exporter is configured by the standard OpenTelemetry variables
(`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_METRIC_EXPORT_INTERVAL`,
`OTEL_RESOURCE_ATTRIBUTES`, `OTEL_SERVICE_NAME`, ...). Set
`OTEL_METRICS_EXPORTER=prometheus` to serve a Prometheus scrape endpoint
instead of pushing OTLP.

Compose-level variables (`.env`, not committed): `BIND_ADDRESS`, `PROXY_PORT`,
`ADMIN_PORT`, `OLLAMA_UPSTREAM`, `GRAFANA_NETWORK`, `OLLAMETRY_MEM_LIMIT`.

## Development

Go and the linters run in containers; nothing is installed on the host.

```sh
make test     # go test -race
make lint     # golangci-lint (.golangci.yml)
make vuln     # govulncheck
make lint-ci  # actionlint + zizmor on .github/workflows
make check    # all of the above; run before every commit (CI runs the same)
make fuzz     # fuzz the parsers, FUZZTIME=30s per target
make cover    # coverage summary
```

Parser tests replay responses recorded from Ollama 0.40.13 in
`internal/dialect/testdata`; proxy tests run the real proxy between an HTTP
client and a fake Ollama.

## Releases

Tagging `vX.Y.Z` publishes `ghcr.io/a1exus/ollametry` for `linux/amd64` and
`linux/arm64` with an SBOM and signed build provenance
(`gh attestation verify oci://ghcr.io/a1exus/ollametry:X.Y.Z -R a1exus/ollametry`),
and creates a GitHub release from that version's `CHANGELOG.md` section.

## License

[MIT](LICENSE)
