# Development

[← README](../README.md)

- [Toolchain](#toolchain)
- [Tests](#tests)
- [Fuzzing](#fuzzing)
- [Continuous integration](#continuous-integration)

[`CLAUDE.md`](../CLAUDE.md) describes the architecture and the API quirks the
decoders handle; [`AGENTS.md`](../AGENTS.md) lists the project's invariants.
Both apply to human contributors too. How to propose changes is in
[`CONTRIBUTING.md`](../CONTRIBUTING.md).

## Toolchain

Go and every linter run in pinned containers; the host needs only `docker`,
`make`, `curl` and `bash`. Module, build and lint caches live in the Docker
volumes `ollametry-gomod`, `ollametry-gobuild` and `ollametry-golangci`.

| Target | Does |
|---|---|
| `make check` | Everything below except fuzzing; run before every commit |
| `make test` | `go test -race` |
| `make cover` | Tests with a per-function coverage summary |
| `make lint` | golangci-lint ([`.golangci.yml`](../.golangci.yml)) |
| `make fmt` | gofmt and goimports |
| `make vuln` | govulncheck on dependencies and the standard library |
| `make tidy` / `make tidy-check` | Tidy `go.mod`; fail if it is not tidy |
| `make lint-ci` | actionlint and zizmor on `.github/workflows` |
| `make dashboard-check` | Validate the Grafana dashboard JSON |
| `make fuzz` | Fuzz the parsers (`FUZZTIME=30s` per target) |

Tool versions are Makefile variables (`GO_VERSION`, `GOLANGCI_VERSION`, ...).
`make help` lists every target.

## Tests

- **Decoders** replay responses recorded from Ollama 0.40.13 in
  `internal/dialect/testdata`. New response shapes get a recorded fixture; cap
  the output with `num_predict` or `max_tokens` so fixtures stay small.
- **Proxy tests** run the real proxy between an HTTP client and a fake Ollama
  and read metrics from an in-memory reader.
- **Telemetry tests** check the bucket views, resource and cardinality limit
  of the real meter provider.

## Fuzzing

| Target | Checks |
|---|---|
| `FuzzFramer` | Framing is identical whether a body arrives whole, split, or byte by byte; no document exceeds the size limit |
| `FuzzDecoders` | No input panics a decoder; `error.type` stays bounded |
| `FuzzModelFromPrefix` | The truncated-body model walk agrees with `encoding/json` on every valid body |

```sh
make fuzz                 # 30s per target
make fuzz FUZZTIME=10m    # a longer campaign
```

A failure writes the input to `testdata/fuzz/<Target>/` in the package;
commit it with the fix so `make test` replays it from then on.

## Continuous integration

| Workflow | Runs on | Does |
|---|---|---|
| `ci.yml` | PRs, pushes to `main` | Three parallel legs: lint and scan (`make ci-lint`), test (`make cover`), fuzz (20s per target); multi-arch image build and a smoke test without Ollama; dependency review on PRs |
| `codeql.yml` | PRs, `main`, weekly | CodeQL `security-and-quality` for Go |
| `scorecard.yml` | `main`, weekly | OpenSSF Scorecard |
| `release.yml` | tags `vX.Y.Z` | See [releases](releases.md) |

CI runs the same `make` targets as you do locally. Its caches are directories
kept by `actions/cache`; only pushes to `main` save them, and pull requests
restore them. Every action is pinned to a commit SHA, and workflows grant
`contents: read` at the top and more per job, each with a comment.
