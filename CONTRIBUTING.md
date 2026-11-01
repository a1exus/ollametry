# Contributing

Thanks for helping. Bug reports, fixes, new API coverage and dashboard
improvements are all welcome.

## Before you start

- For anything bigger than a small fix, open an issue first so we can agree
  on the approach.
- Security issues go through [SECURITY.md](SECURITY.md), not public issues.

## Development

Only `docker`, `make`, `curl` and `bash` are needed on the host; Go and the
linters run in pinned containers.

```sh
make check   # golangci-lint, workflow lint, go test -race, govulncheck
make up      # run against a local Ollama (needs ~/opt/grafana's network, see README)
make smoke MODEL=<an installed model>
```

[`AGENTS.md`](AGENTS.md) lists the project's invariants and conventions, and
[`CLAUDE.md`](CLAUDE.md) explains the architecture and the API quirks the
decoders handle. Both apply to human contributors too.

## Pull requests

- One logical change per pull request, in small commits with imperative
  subjects of at most 72 characters.
- Add or update tests. New response shapes get a fixture recorded from a real
  Ollama in `internal/dialect/testdata` (cap the output with `num_predict` or
  `max_tokens` so fixtures stay small).
- Add a line to `CHANGELOG.md` under `## [Unreleased]`
  ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)).
- Changing a metric name or attribute is a breaking change: update the
  dashboard, README and CHANGELOG in the same pull request.
- `make check` must pass; CI runs the same targets.

## Releases

Maintainers move the `Unreleased` entries under a new version heading in
`CHANGELOG.md`, then push a tag `vX.Y.Z`. The release workflow publishes the
multi-arch image to `ghcr.io/a1exus/ollametry` with SBOM and provenance and
creates the GitHub release from that changelog section.
