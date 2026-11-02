# Development

[← README](../README.md)

- [Toolchain](#toolchain)
- [Testing](#testing)
- [Fuzzing](#fuzzing)
- [Performance](#performance)
- [Continuous integration](#continuous-integration)

[`CLAUDE.md`](../CLAUDE.md) describes the architecture and the API quirks the
decoders handle; [`AGENTS.md`](../AGENTS.md) lists the project's invariants.
Both apply to human contributors too. How to propose changes is in
[`CONTRIBUTING.md`](../CONTRIBUTING.md).

## Toolchain

Go and every linter run in pinned containers; the host needs only `docker`,
`make`, `curl` and `bash`. Module, build and lint caches live in the Docker
volumes `ollametry-gomod`, `ollametry-gobuild` and `ollametry-golangci`.

| Develop on | Setup |
| --- | --- |
| macOS | Docker Desktop (or another Docker), plus `make` from the Xcode command-line tools |
| Linux | [Docker Engine](https://docs.docker.com/engine/install/), `make` |
| Windows | [WSL 2](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-containers) with Docker Desktop or Docker Engine; run `make` in the WSL shell with the repository on the Linux file system |

The code only ships for Linux, in the container, so tests run on Linux: in the
pinned golang container locally, and on Linux runners in CI.

| Target | Does |
| --- | --- |
| `make check` | Lint, unit and integration tests, scans; run before every commit |
| `make test` | Unit and integration tests (`go test -race -tags integration`) |
| `make test-unit` / `make test-integration` | One layer at a time |
| `make cover` | Unit and integration tests with a per-function coverage summary |
| `make e2e` / `make e2e-hermetic` | End-to-end tests (see [Testing](#testing)) |
| `make lint` | golangci-lint ([`.golangci.yml`](../.golangci.yml)) |
| `make fmt` | gofmt and goimports |
| `make vuln` | govulncheck on dependencies and the standard library |
| `make tidy` / `make tidy-check` | Tidy `go.mod`; fail if it is not tidy |
| `make versions-check` | Fail if `go.mod`, the Makefile and the Dockerfile name different Go versions |
| `make lint-ci` | actionlint and zizmor on `.github/workflows` |
| `make openapi-check` | Spectral lint of [`api/openapi.yaml`](../api/openapi.yaml); `TestHandlerMatchesOpenAPI` checks it against the real handlers |
| `make docs-check` | markdownlint ([`.markdownlint-cli2.yaml`](../.markdownlint-cli2.yaml)) and an offline check of every relative link and anchor (lychee) |
| `make dashboard-check` | Validate the Grafana dashboard JSON |
| `make rules-check` | `promtool check rules` and `promtool test rules` on [`deploy/prometheus`](../deploy/prometheus) |
| `make notices` / `make notices-check` | Regenerate the Go module table in [`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md); fail if it misses a dependency |
| `make fuzz` | Fuzz the parsers (`FUZZTIME=30s` per target) |
| `make bench` / `make benchstat` | Benchmarks into `bench.txt`; compare two runs (see [Performance](#performance)) |

Tool versions are Makefile variables (`GO_VERSION`, `GOLANGCI_VERSION`, ...).
`make help` lists every target.

## Testing

Three layers, from fast and isolated to slow and real:

| Layer | Scope | Where | Run with |
| --- | --- | --- | --- |
| **Unit** | One function or package: decoders, framer, routes, config, meter provider, poller state, model extraction | `*_test.go`, no build tag | `make test-unit` (seconds) |
| **Integration** | Several modules over real HTTP with fake externals: the proxy between a client and a fake Ollama; the poller against a fake Ollama; the whole binary (`run()`) exporting OTLP/HTTP to a fake collector that decodes the protobuf | `*_integration_test.go`, `//go:build integration` | `make test-integration` (seconds) |
| **End-to-end** | The deployed system from a client's side: requests through the ollametry container to a real Ollama in every API family, then the resulting series in otel-lgtm's Prometheus | `test/e2e`, `//go:build e2e` | `make e2e` or `make e2e-hermetic` (minutes) |

- **Fakes replay real responses.** Decoder, proxy and binary tests use
  responses recorded from Ollama 0.40.13 in `internal/dialect/testdata` and
  `internal/ollama/testdata`. New response shapes get a recorded fixture; cap
  the output with `num_predict` or `max_tokens` so fixtures stay small.
- **`make e2e`** runs against your running stack (`make up`, the host
  Ollama, `~/opt/grafana`) with `E2E_MODEL` (default `granite4.2:8b`). The
  runner joins the `grafana_default` network to reach Prometheus.
- **`make e2e-hermetic`** brings up a throwaway stack from
  [`test/e2e/compose.yaml`](../test/e2e/compose.yaml): Ollama with
  `smollm2:135m`, otel-lgtm, and ollametry built from the checkout. It tears
  the stack down afterwards and prints ollametry's logs on failure. It needs
  no host Ollama, so it also suits a machine without one.
- **E2E runs locally only, never in GitHub CI.** It needs a real Ollama and
  multi-GB images pulled from external registries, which makes it slow and
  dependent on services outside the repository. Pull requests get the unit
  and integration layers, which cover the assembled binary and real OTLP;
  run `make e2e` before merging changes that affect what reaches Prometheus
  (the pull request template asks for it).
- E2E tests compare series one by one, never summed, and wait for exports
  instead of reading once: series from a replaced container linger for 5
  minutes, and metrics arrive on the export interval.

## Fuzzing

| Target | Checks |
| --- | --- |
| `FuzzFramer` | Framing is identical whether a body arrives whole, split, or byte by byte; no document exceeds the size limit |
| `FuzzDecoders` | No input panics a decoder; `error.type` stays bounded |
| `FuzzModelFromPrefix` | The truncated-body model walk agrees with `encoding/json` on every valid body |

```sh
make fuzz                 # 30s per target
make fuzz FUZZTIME=10m    # a longer campaign
```

Fuzzing is not a pull request gate: it explores random inputs, so a find would
fail an unrelated change. Pull requests replay the seed corpus and every saved
failure through `make test`, deterministically. The nightly
[`fuzz.yml`](../.github/workflows/fuzz.yml) workflow fuzzes each target for
10 minutes in parallel (longer from *Run workflow*) and uploads failing inputs
as an artifact.

A failure writes the input to `testdata/fuzz/<Target>/` in the package;
commit it with the fix so `make test` replays it from then on.

## Performance

What ollametry adds to each request, from `make bench` on an Apple M2
(Docker Desktop, linux/arm64). `BenchmarkProxy` sends a recorded response
through the whole proxy from memory; `BenchmarkBaseline` relays the same
bytes without ollametry.

| Response | Through ollametry | Plain relay | Added |
| --- | --- | --- | --- |
| Native chat, streamed (13 records) | 39.5 µs, 30.1 KiB | 2.0 µs, 7.9 KiB | 37 µs, 22 KiB |
| Native chat, single JSON | 24.2 µs, 24.7 KiB | 1.6 µs, 6.2 KiB | 23 µs, 18 KiB |
| OpenAI chat, streamed | 46.1 µs, 30.6 KiB | 2.3 µs, 9.3 KiB | 44 µs, 21 KiB |
| Anthropic messages, streamed | 38.0 µs, 26.7 KiB | 2.0 µs, 8.1 KiB | 36 µs, 19 KiB |

Against model latencies of hundreds of milliseconds to minutes, that is far
below 0.1 %. Most of it is the fixed cost of recording the metrics; each
streamed record adds about 2 allocations (decoding its JSON).
`TestProxyAllocationsPerRecord` fails if that grows by one, which is the
regression that would scale with long generations; timings are too noisy on
shared CI runners to gate on.

To measure a change:

```sh
make bench && mv bench.txt old.txt    # on the base commit
make bench                             # on your change
make benchstat                         # compares old.txt with bench.txt
```

`make test-unit` (and CI) runs every benchmark once, so they keep working.

## Continuous integration

| Workflow | Runs on | Does |
| --- | --- | --- |
| `ci.yml` | PRs, pushes to `main` | Parallel legs: lint and scan (`make ci-lint`, including `openapi-check`, `docs-check`, `rules-check` and `notices-check`), unit tests (`make test-unit`), integration tests with coverage (`make cover`); multi-arch image build and a smoke test; dependency review on PRs |
| `fuzz.yml` | Nightly, manual | Each fuzz target for 10 minutes, failing inputs uploaded |
| `codeql.yml` | PRs, `main`, weekly | CodeQL `security-and-quality` for Go |
| `scorecard.yml` | `main`, weekly | OpenSSF Scorecard |
| `release.yml` | tags `vX.Y.Z` | See [releases](releases.md) |

CI runs the same `make` targets as you do locally. Its caches are directories
kept by `actions/cache`; only pushes to `main` save them, and pull requests
restore them. Every action is pinned to a commit SHA, and workflows grant
`contents: read` at the top and more per job, each with a comment.
