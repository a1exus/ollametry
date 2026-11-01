# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Transparent reverse proxy for Ollama (`ollametry`) that relays requests and streams unchanged, flushes every write, and cancels the upstream request when the client hangs up.
- Response decoding for Ollama's native API (`/api/generate`, `/api/chat`, `/api/embed`, `/api/embeddings`), the OpenAI-compatible API (`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`), the Anthropic Messages API (`/v1/messages`) and the Responses API (`/v1/responses`), streamed or not.
- OpenTelemetry GenAI metrics: `gen_ai.client.operation.duration`, `gen_ai.client.token.usage`, `gen_ai.server.time_to_first_token`, `gen_ai.server.time_per_output_token`; plus `ollama.prompt.cached_tokens`, `ollama.model.load.duration`, `ollama.prompt.eval.duration`, `ollama.inference.active`.
- HTTP server metrics for every proxied route: `http.server.request.duration`, `http.server.active_requests`, with low-cardinality `http.route` templates.
- `error.type` classification: HTTP status, API error types reported inside a stream, `incomplete`, `canceled`, `upstream_error`.
- Poller for `/api/version`, `/api/ps` and `/api/tags`: `ollama.up`, per-model memory, GPU memory, context length and keep-alive remaining, installed model count and disk use.
- Configuration by flags and `OLLAMETRY_*` variables; exporter configured by standard `OTEL_*` variables (OTLP by default, Prometheus or console optional); Go runtime metrics.
- `/livez` and `/readyz` on a separate admin listener; `ollametry healthcheck` subcommand for the distroless image.
- Distroless, non-root, read-only container on the `grafana_default` network, ports bound to `127.0.0.1` (11435 proxy, 11436 admin).
- Grafana dashboard `deploy/grafana/ollametry.json`, provisioned by `~/opt/grafana` (`make dashboards`).
- Makefile running Go 1.27.3, golangci-lint v2.15.0 and govulncheck v1.9.0 in containers: `test`, `lint`, `fmt`, `vuln`, `check`, `cover`, `up`, `down`, `health`, `smoke`.
- Tests against responses recorded from Ollama 0.40.13.
- `README.md`, `AGENTS.md`, `CLAUDE.md`, `CHANGELOG.md`.
- MIT license (`LICENSE`), also declared on the image as `org.opencontainers.image.licenses`.
- GitHub Actions: CI (`make` lint, race tests with coverage, govulncheck, workflow lint, multi-arch image build and smoke test, dependency review), CodeQL, OpenSSF Scorecard, and a tag-triggered release publishing the image to GHCR with SBOM, provenance and attestation. All actions pinned to commit SHAs.
- `make lint-ci` (actionlint 1.7.12, zizmor 1.30.1), now part of `make check`.
- Dependabot for Go modules, GitHub Actions and the base image, weekly with a 14-day cooldown.
- `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), issue forms, pull request template, `CODEOWNERS`, `.editorconfig`; README badges.

### Security

- `golang.org/x/net` v0.59.0 → v0.60.0 (GO-2026-6603, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6617).
