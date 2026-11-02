# AGENTS.md

Operating rules for AI agents working in this repo. Architecture details and rationale live in [`CLAUDE.md`](CLAUDE.md); user-facing docs are [`README.md`](README.md) and [`docs/`](docs/).

## Security and privacy

- Never log or record prompt or response content. Metrics and logs carry counts, durations, model names and routes only.
- Never create or modify `.env`; ask the user and give them the exact line.
- Never read Grafana credentials.

## Off-limits

| Path / action | Reason |
|---|---|
| `.env` | User-owned local config |
| `~/opt/grafana`, `~/opt/traefik` | Separate repos with their own AGENTS.md; change them only on explicit request. Dashboard JSON lives here and `~/opt/grafana` pulls it (`make dashboards`) |
| The host Ollama configuration (`launchctl setenv`, restarting the app) | Changes how every client reaches Ollama; only on explicit request |
| `git push`, publishing tags or images | Only when the user explicitly asks |

## Invariants

- **The proxy is transparent.** Request and response bytes pass through unchanged (the one exception: `Accept-Encoding` is dropped upstream on inference routes, and the client gets an uncompressed body). Streams flush every write. A metrics failure never fails a request.
- **Bounded cardinality.** `http.route` only from the known-route table; model names over 256 bytes become `_OTHER`; error types are status codes, a fixed set, or API types matching `^[a-z][a-z0-9_]{0,63}$`.
- **Metric names follow OTel semconv** where one exists (`gen_ai.*`, `http.server.*`); project metrics are `ollama.*`. Renaming a metric is a breaking change: update the dashboard, `docs/metrics.md` and CHANGELOG together.
- **Published ports bind to `${BIND_ADDRESS:-127.0.0.1}`.** Images and tool versions are pinned; never `latest`.
- **Toolchains run in containers** via `make`. Host tools: `docker`, `make`, `curl`, `bash`.
- **GitHub Actions are pinned to full commit SHAs** (version in a trailing comment), at least 14 days old, with least-privilege `permissions` commented per job. Untrusted event data never goes into `run:` directly; pass it through `env:`.

## Code conventions

- Go: `fmt.Errorf("context: %w", err)`; no `panic` outside re-raising `http.ErrAbortHandler`; `context.Context` first; `any`, not `interface{}`; imports grouped stdlib → external → `github.com/a1exus/ollametry` (enforced by `make fmt`).
- Tests are table-driven with `t.Run`, use only the standard library and the OTel SDK's `ManualReader`, and new response shapes get a recorded fixture in `internal/dialect/testdata` (record with `num_predict`/`max_tokens` capped so fixtures stay small).
- Errors name the component and what was expected (`ollama-url: "ftp://x" must be an absolute http or https URL`).

## Verification (run after every change)

```sh
make check          # golangci-lint, actionlint + zizmor, go test -race, govulncheck
make up             # waits for the container healthcheck
make health
make smoke MODEL=<an installed model>
```

Dashboard changes: run every query against Prometheus (see CLAUDE.md), `make -C ~/opt/grafana dashboards`, and look at the rendered dashboard before committing.

## Changes and commits

- Every change adds a line to `CHANGELOG.md` under `## [Unreleased]` ([Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/)). Dependency and image bumps go under `Changed` as old → new; vulnerability fixes under `Security`.
- Commit subjects are imperative, ≤ 72 characters; small focused commits; work on a branch, never commit to `main`.
- If a change alters metrics, ports, variables or make targets, update the matching `docs/` page (metrics, configuration, operations) and `CLAUDE.md` in the same change.
