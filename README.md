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

```text
 clients ──► ollametry :11435 ──► Ollama :11434 (on the host: macOS, Linux or Windows)
                 │
                 ├─ per request: gen_ai.* and http.server.* metrics
                 └─ every 15 s:  /api/ps, /api/tags → ollama.* gauges
                 │
                 ├─ :11436/metrics ◄── Prometheus scrape
                 └─ OTLP/HTTP ──► otel-lgtm :4318 ──► Prometheus ──► Grafana
```

## Contents

- [Platforms](#platforms)
- [Quick start](#quick-start)
- [Documentation](#documentation)
- [Supported APIs](#supported-apis)
- [Contributing and security](#contributing-and-security)
- [License](#license)

## Platforms

ollametry is one static binary with no dependencies. Run it natively next to
Ollama, or in a container.

| OS | Binary | Run as | Container | Tested in CI |
| --- | --- | --- | --- | --- |
| macOS | `arm64`, `amd64` | [launchd agent](docs/install.md#macos-service-launchd) | [Docker Desktop, Apple `container`](docs/containers.md) | Unit and integration tests on macOS |
| Linux | `amd64`, `arm64` | [systemd unit](docs/install.md#linux-service-systemd) | [Docker Engine](docs/containers.md#linux-docker-engine), with NVIDIA GPUs | All checks, image build and smoke test |
| Windows | `amd64`, `arm64` (`ollametry.exe`) | [Task Scheduler](docs/install.md#windows-task-scheduler) | [WSL 2](docs/containers.md#windows-wsl-2) | Unit and integration tests on Windows |

On macOS, and on Windows without an NVIDIA GPU, Ollama itself should run on
the host: containers there get no GPU ([why](docs/containers.md#where-ollama-should-run-gpu-access)).

## Quick start

**Native, any OS:** install a [release binary](docs/install.md#binary), then
run it next to Ollama:

```sh
ollametry                             # proxy on :11435, /metrics on :11436
curl -s http://127.0.0.1:11436/readyz # ok once Ollama answers
```

**This repository's setup**, with Docker Compose next to the
`~/opt/grafana` stack (Docker Compose v2.24+, `make`, `curl`, `bash`; on
Windows, from WSL 2):

```sh
make up                               # docker compose up -d --build, wait until healthy
make health                           # Ollama, the proxy, readiness
make smoke MODEL=granite4.2:8b        # one request through the proxy
make -C ~/opt/grafana dashboards      # provision the dashboard into Grafana
```

Then point clients at `http://127.0.0.1:11435` instead of `:11434`
([more clients](docs/clients.md)):

```sh
OLLAMA_HOST=127.0.0.1:11435 ollama run granite4.2:8b
export OPENAI_BASE_URL=http://127.0.0.1:11435/v1
export ANTHROPIC_BASE_URL=http://127.0.0.1:11435
```

```powershell
$env:OLLAMA_HOST = "127.0.0.1:11435"; ollama run granite4.2:8b
$env:OPENAI_BASE_URL = "http://127.0.0.1:11435/v1"
$env:ANTHROPIC_BASE_URL = "http://127.0.0.1:11435"
```

Prometheus can scrape `http://127.0.0.1:11436/metrics`, and with the Compose
setup the dashboard is at `https://grafana.localhost/d/ollametry` (folder
"ollama"). `make logs` follows the logs, `make down` stops everything, and
`make help` lists every target.

## Documentation

| Guide | Covers |
| --- | --- |
| [Install](docs/install.md) | Binaries for macOS, Linux and Windows; running as a launchd, systemd or Task Scheduler service; the container image; `go install` |
| [Container environments](docs/containers.md) | Linux Docker Engine, macOS Docker Desktop or Apple `container`, Windows WSL 2; GPU access and where Ollama should run; NVIDIA GPUs |
| [Clients](docs/clients.md) | Pointing the Ollama CLI and OpenAI/Anthropic SDKs at the proxy; measuring every client |
| [Metrics](docs/metrics.md) | Every metric and attribute, `error.type`, limits, writing PromQL against OTLP series |
| [Prometheus exporter](docs/prometheus.md) | Scraping `/metrics`, running without otel-lgtm, how it compares with OTLP, alerting and recording rules |
| [Configuration](docs/configuration.md) | `OLLAMETRY_*`, `OTEL_*` and Compose variables |
| [Operations](docs/operations.md) | Compose lifecycle targets, health endpoints, dashboard provisioning, upgrading, troubleshooting |
| [Admin API reference](api/openapi.yaml) | OpenAPI 3.1 for `/livez`, `/readyz` and `/metrics` on the admin listener |
| [Development](docs/development.md) | Containerized toolchain, tests, fuzzing, CI |
| [Releases](docs/releases.md) | Publishing, verifying and running released images |

Also: [`CLAUDE.md`](CLAUDE.md) (architecture and verified API quirks),
[`AGENTS.md`](AGENTS.md) (rules for AI agents), [`CHANGELOG.md`](CHANGELOG.md).

## Supported APIs

| API | Endpoints | Streaming |
| --- | --- | --- |
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
