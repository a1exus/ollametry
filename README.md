# ollametry

[![CI](https://github.com/a1exus/ollametry/actions/workflows/ci.yml/badge.svg)](https://github.com/a1exus/ollametry/actions/workflows/ci.yml)
[![CodeQL](https://github.com/a1exus/ollametry/actions/workflows/codeql.yml/badge.svg)](https://github.com/a1exus/ollametry/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/a1exus/ollametry/badge)](https://scorecard.dev/viewer/?uri=github.com/a1exus/ollametry)
[![Go](https://img.shields.io/github/go-mod/go-version/a1exus/ollametry)](go.mod)
[![License: MIT](https://img.shields.io/github/license/a1exus/ollametry)](LICENSE)

An **Ollama metrics exporter** for Prometheus and OpenTelemetry. ollametry is a
transparent reverse proxy for [Ollama](https://ollama.com) that measures token
usage, latency, time to first token and decode speed per model, error rates,
and the models Ollama holds in memory. Metrics follow the OpenTelemetry
[GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
and leave two ways, with a ready-made Grafana dashboard for either:

- **Prometheus scrape**: `/metrics` on `:11436`, for any Prometheus-compatible
  scraper ([Prometheus exporter](docs/prometheus.md)).
- **OTLP push**: to [`grafana/otel-lgtm`](https://github.com/grafana/docker-otel-lgtm)
  (`~/opt/grafana`) or any OpenTelemetry backend.

Ollama has no metrics endpoint, so ollametry sits in front of it and reads the
usage statistics Ollama already returns in every response. Requests and
responses pass through byte for byte, streaming included.

```
 clients ──► ollametry :11435 ──► Ollama :11434 (macOS host)
                 │
                 ├─ per request: gen_ai.* and http.server.* metrics
                 └─ every 15 s:  /api/ps, /api/tags → ollama.* gauges
                 │
                 ├─ :11436/metrics ◄── Prometheus scrape
                 └─ OTLP/HTTP ──► otel-lgtm :4318 ──► Prometheus ──► Grafana
```

## Contents

- [Prerequisites](#prerequisites)
- [Quick start](#quick-start)
- [Documentation](#documentation)
- [Supported APIs](#supported-apis)
- [Contributing and security](#contributing-and-security)
- [License](#license)

## Prerequisites

- Ollama running on the host (`ollama serve` or the macOS app) on `127.0.0.1:11434`.
- The `~/opt/grafana` stack running (`make up` there); ollametry joins its
  `grafana_default` network.
- Docker Compose v2.24 or newer. Host tools: `docker`, `make`, `curl`, `bash`.

## Quick start

```sh
make up                               # docker compose up -d --build, wait until healthy
make health                           # Ollama, the proxy, readiness
make smoke MODEL=granite4.2:8b        # one request through the proxy
make -C ~/opt/grafana dashboards      # provision the dashboard into Grafana
```

Without Docker, run a [release binary](docs/install.md) as a macOS launchd
service next to the Ollama app.

Then point clients at `http://127.0.0.1:11435` instead of `:11434`:

```sh
OLLAMA_HOST=127.0.0.1:11435 ollama run granite4.2:8b
export OPENAI_BASE_URL=http://127.0.0.1:11435/v1
export ANTHROPIC_BASE_URL=http://127.0.0.1:11435
```

Prometheus can scrape `http://127.0.0.1:11436/metrics`, and the dashboard is at
`https://grafana.localhost/d/ollametry` (folder "ollama").
`make logs` follows the logs, `make down` stops everything, and `make help`
lists every target.

## Documentation

| Guide | Covers |
|---|---|
| [Install](docs/install.md) | Binaries for macOS and Linux, a launchd service, the container image, `go install` |
| [Clients](docs/clients.md) | Pointing the Ollama CLI and OpenAI/Anthropic SDKs at the proxy; measuring every client |
| [Metrics](docs/metrics.md) | Every metric and attribute, `error.type`, limits, writing PromQL against OTLP series |
| [Prometheus exporter](docs/prometheus.md) | Scraping `/metrics`, running without otel-lgtm, how it compares with OTLP, alerting and recording rules |
| [Configuration](docs/configuration.md) | `OLLAMETRY_*`, `OTEL_*` and Compose variables |
| [Operations](docs/operations.md) | Compose lifecycle targets, health endpoints, dashboard provisioning, upgrading, troubleshooting |
| [Development](docs/development.md) | Containerized toolchain, tests, fuzzing, CI |
| [Releases](docs/releases.md) | Publishing, verifying and running released images |

Also: [`CLAUDE.md`](CLAUDE.md) (architecture and verified API quirks),
[`AGENTS.md`](AGENTS.md) (rules for AI agents), [`CHANGELOG.md`](CHANGELOG.md).

## Supported APIs

| API | Endpoints | Streaming |
|---|---|---|
| Ollama native | `/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings` | NDJSON |
| OpenAI-compatible | `/v1/chat/completions`, `/v1/completions`, `/v1/embeddings` | SSE |
| Anthropic Messages | `/v1/messages` | SSE |
| OpenAI Responses | `/v1/responses` | SSE |

Every other path is relayed unchanged and counted in the HTTP metrics only.

## Contributing and security

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md). Report vulnerabilities privately as
described in [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE). Third-party software and its licenses are listed in
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
