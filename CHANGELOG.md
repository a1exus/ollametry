# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

The first release.

### Added

#### Proxy

- Transparent reverse proxy for Ollama: requests and responses pass through
  byte for byte, streams are flushed on every write, and the upstream request
  is canceled when the client hangs up.
- Usage decoding for Ollama's native API (`/api/generate`, `/api/chat`,
  `/api/embed`, `/api/embeddings`), the OpenAI-compatible API
  (`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`), the
  Anthropic Messages API (`/v1/messages`) and the Responses API
  (`/v1/responses`), streamed or not.
- The request model is read like Ollama reads it (`encoding/json`:
  case-insensitive keys, last duplicate wins), also from bodies larger than
  `OLLAMETRY_MAX_INSPECT_BYTES`.
- Protocol upgrades (`101 Switching Protocols`) pass through untouched.

#### Metrics

- OpenTelemetry GenAI metrics: `gen_ai.client.operation.duration`,
  `gen_ai.client.token.usage`, `gen_ai.server.time_to_first_token` and
  `gen_ai.server.time_per_output_token`, plus `ollama.prompt.cached_tokens`,
  `ollama.model.load.duration`, `ollama.prompt.eval.duration` and
  `ollama.inference.active`.
- HTTP server metrics for every proxied route, with low-cardinality
  `http.route` templates.
- `error.type` for failures: the HTTP status, the API's own error type for
  errors inside a stream, `incomplete`, `canceled` or `upstream_error`.
- Gauges from polling Ollama: `ollama.up`, `ollama.build.info` (with
  `ollama.version`), memory, GPU memory, context length and keep-alive per
  loaded model, and installed models with their disk use.
- Two ways out, from one meter provider so the series match: OTLP push,
  configured by the standard `OTEL_*` variables, and a Prometheus `/metrics`
  endpoint on the admin listener (`OLLAMETRY_PROMETHEUS`, on by default).
- Bounded cardinality: model names Ollama does not have, or longer than 256
  bytes, are recorded as `_OTHER`; a per-metric cardinality limit applies.

#### Operations

- Admin listener with `/livez`, `/readyz` and `/metrics`, specified in
  `api/openapi.yaml` (OpenAPI 3.1).
- Graceful shutdown that waits for in-flight requests, records their metrics
  and flushes a final export.
- `ollametry version` and `ollametry healthcheck` subcommands.
- Grafana dashboard (`deploy/grafana/ollametry.json`) and Prometheus alerting
  and recording rules (`deploy/prometheus/ollametry-rules.yml`): Ollama down,
  ollametry absent, server error ratio, slow time to first token, model
  thrashing, models offloaded to CPU, cardinality overflow.
- Log lines in JSON or text, with client-controlled values bounded in size.

#### Platforms and distribution

- Binaries for macOS, Linux (`.tar.gz`) and Windows (`.zip`), each for
  `arm64` and `amd64`: reproducible archives with `SHA256SUMS` and signed build
  provenance.
- Multi-arch container image on `ghcr.io/a1exus/ollametry`: distroless,
  non-root, with SBOM, provenance and the license notices in
  `/usr/share/doc/ollametry/`.
- Service setups: a launchd agent for macOS, a hardened systemd unit for
  Linux, and Task Scheduler steps for Windows.
- Docker Compose setup with `make` lifecycle targets (`up`, `down`, `logs`,
  `health`, ...), and guidance per OS and container runtime, including GPU
  access and NVIDIA containers.

#### Quality and supply chain

- Unit and integration tests on Linux, macOS and Windows in CI; end-to-end
  tests against a real Ollama and otel-lgtm, run locally; nightly fuzzing of
  the parsers; benchmarks with a guard on allocations per streamed record.
- Static analysis on every pull request: golangci-lint (with gosec), CodeQL,
  govulncheck, actionlint and zizmor, Spectral for the OpenAPI document,
  promtool for the rules, and markdownlint and an offline link check for the
  documentation.
- Supply chain: actions pinned to commit SHAs and base images to digests,
  Dependabot with a 14-day cooldown, OpenSSF Scorecard, a protected `main`
  branch, and a release workflow with a dry-run mode.

#### Documentation

- README with platforms and a quick start; guides in `docs/` for installing,
  container environments, clients, metrics, the Prometheus exporter,
  configuration, operations, development and releases.
- `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `THIRD_PARTY_NOTICES.md`, and `.bestpractices.json` for the OpenSSF Best
  Practices badge.
