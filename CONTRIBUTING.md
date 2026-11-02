# Contributing

Thanks for helping. Bug reports, fixes, new API coverage and dashboard
improvements are all welcome.

## Before you start

- For anything bigger than a small fix, open an issue first so we can agree
  on the approach.
- Security issues go through [SECURITY.md](SECURITY.md), not public issues.
- Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development

Only `docker`, `make`, `curl` and `bash` are needed on the host; Go and the
linters run in pinned containers.

```sh
make check   # golangci-lint, workflow lint, OpenAPI, docs, dashboard and rule checks, tests, govulncheck
make fuzz    # fuzz the parsers (FUZZTIME=30s per target)
make up      # run against a local Ollama (needs ~/opt/grafana's network, see docs/operations.md)
make smoke MODEL=<an installed model>
```

[`AGENTS.md`](AGENTS.md) lists the project's invariants and conventions, and
[`docs/development.md`](docs/development.md) covers the toolchain, tests and CI;
[`CLAUDE.md`](CLAUDE.md) explains the architecture and the API quirks the
decoders handle. Both apply to human contributors too.

## Pull requests

- One logical change per pull request, in small commits with imperative
  subjects of at most 72 characters.
- Add or update tests at the right layer (see
  [Testing](docs/development.md#testing)): unit tests for logic, integration
  tests for anything that crosses HTTP or OTLP, E2E when the user-visible
  result in Prometheus changes. New response shapes get a fixture recorded from
  a real Ollama in `internal/dialect/testdata` (cap the output with
  `num_predict` or `max_tokens` so fixtures stay small).
- A new dependency needs `make notices`; a new tool, service or action needs a
  row in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
- Add a line to `CHANGELOG.md` under `## [Unreleased]`
  ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)).
- Changing a metric name or attribute is a breaking change: update the
  dashboard, `docs/metrics.md` and CHANGELOG in the same pull request.
- `make check` must pass; CI runs the same checks plus `tidy-check`,
  `versions-check` and `notices-check` (`make ci-lint`).
- `main` has linear history and accepts only rebase merges. Keep your branch
  current with `git rebase origin/main` (not by merging `main` in); the
  branch must be up to date and every required check green before it can
  merge. There is no bypass, for maintainers either.

## Releases

Maintainers move the `Unreleased` entries under a new version heading in
`CHANGELOG.md`, then push a tag `vX.Y.Z`. The release workflow publishes the
multi-arch image to `ghcr.io/a1exus/ollametry` with SBOM and provenance and
creates the GitHub release from that changelog section. Run a
[dry run](docs/releases.md#publishing) first.
