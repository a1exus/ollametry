# Third-party notices

ollametry is licensed under the [MIT License](LICENSE). It builds on the
third-party software below, which remains under its own licenses. Each name
links to the project and its license.

- [Distributed with ollametry](#distributed-with-ollametry)
  - [Go standard library and runtime](#go-standard-library-and-runtime)
  - [Go modules](#go-modules)
  - [Container base image](#container-base-image)
- [Used but not distributed](#used-but-not-distributed)
  - [Services ollametry works with](#services-ollametry-works-with)
  - [Specifications](#specifications)
  - [Build and development tools](#build-and-development-tools)
  - [GitHub Actions](#github-actions)

## Distributed with ollametry

These are compiled into the `ollametry` binary or shipped in the container
image. The image carries this file and [`LICENSE`](LICENSE) in
`/usr/share/doc/ollametry/`.

### Go standard library and runtime

| Component | License |
|---|---|
| [Go](https://go.dev) standard library and runtime | [BSD-3-Clause](https://go.dev/LICENSE) |

### Go modules

The packages linked into the binary, as reported by
[go-licenses](https://github.com/google/go-licenses). The OpenTelemetry
modules list two licenses because their license file adds BSD-3-Clause
notices for code they include. Regenerate with `make notices`; CI fails when
this table misses a dependency.

<!-- BEGIN go-licenses -->
| Package | License |
|---|---|
| [`github.com/beorn7/perks/quantile`](https://pkg.go.dev/github.com/beorn7/perks/quantile?tab=licenses) | MIT |
| [`github.com/cenkalti/backoff/v5`](https://pkg.go.dev/github.com/cenkalti/backoff/v5?tab=licenses) | MIT |
| [`github.com/cespare/xxhash/v2`](https://pkg.go.dev/github.com/cespare/xxhash/v2?tab=licenses) | MIT |
| [`github.com/go-logr/logr`](https://pkg.go.dev/github.com/go-logr/logr?tab=licenses) | Apache-2.0 |
| [`github.com/go-logr/stdr`](https://pkg.go.dev/github.com/go-logr/stdr?tab=licenses) | Apache-2.0 |
| [`github.com/google/uuid`](https://pkg.go.dev/github.com/google/uuid?tab=licenses) | BSD-3-Clause |
| [`github.com/grpc-ecosystem/grpc-gateway/v2`](https://pkg.go.dev/github.com/grpc-ecosystem/grpc-gateway/v2?tab=licenses) | BSD-3-Clause |
| [`github.com/munnerz/goautoneg`](https://pkg.go.dev/github.com/munnerz/goautoneg?tab=licenses) | BSD-3-Clause |
| [`github.com/prometheus/client_golang/internal/github.com/golang/gddo/httputil`](https://pkg.go.dev/github.com/prometheus/client_golang/internal/github.com/golang/gddo/httputil?tab=licenses) | BSD-3-Clause |
| [`github.com/prometheus/client_golang/prometheus`](https://pkg.go.dev/github.com/prometheus/client_golang/prometheus?tab=licenses) | Apache-2.0 |
| [`github.com/prometheus/client_model/go`](https://pkg.go.dev/github.com/prometheus/client_model/go?tab=licenses) | Apache-2.0 |
| [`github.com/prometheus/common`](https://pkg.go.dev/github.com/prometheus/common?tab=licenses) | Apache-2.0 |
| [`github.com/prometheus/otlptranslator`](https://pkg.go.dev/github.com/prometheus/otlptranslator?tab=licenses) | Apache-2.0 |
| [`github.com/prometheus/procfs`](https://pkg.go.dev/github.com/prometheus/procfs?tab=licenses) | Apache-2.0 |
| [`go.opentelemetry.io/auto/sdk`](https://pkg.go.dev/go.opentelemetry.io/auto/sdk?tab=licenses) | Apache-2.0 |
| [`go.opentelemetry.io/contrib/bridges/prometheus`](https://pkg.go.dev/go.opentelemetry.io/contrib/bridges/prometheus?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/contrib/exporters/autoexport`](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/contrib/instrumentation/runtime`](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel`](https://pkg.go.dev/go.opentelemetry.io/otel?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlptrace`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/prometheus`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/prometheus?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/stdout/stdoutlog`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/stdout/stdoutlog?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/stdout/stdoutmetric`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/stdout/stdoutmetric?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/exporters/stdout/stdouttrace`](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/stdout/stdouttrace?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/log`](https://pkg.go.dev/go.opentelemetry.io/otel/log?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/metric`](https://pkg.go.dev/go.opentelemetry.io/otel/metric?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/sdk`](https://pkg.go.dev/go.opentelemetry.io/otel/sdk?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/sdk/log`](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/log?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/sdk/metric`](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/otel/trace`](https://pkg.go.dev/go.opentelemetry.io/otel/trace?tab=licenses) | Apache-2.0, BSD-3-Clause |
| [`go.opentelemetry.io/proto/otlp`](https://pkg.go.dev/go.opentelemetry.io/proto/otlp?tab=licenses) | Apache-2.0 |
| [`golang.org/x/net`](https://pkg.go.dev/golang.org/x/net?tab=licenses) | BSD-3-Clause |
| [`golang.org/x/sys/unix`](https://pkg.go.dev/golang.org/x/sys/unix?tab=licenses) | BSD-3-Clause |
| [`golang.org/x/text`](https://pkg.go.dev/golang.org/x/text?tab=licenses) | BSD-3-Clause |
| [`google.golang.org/genproto/googleapis/api/httpbody`](https://pkg.go.dev/google.golang.org/genproto/googleapis/api/httpbody?tab=licenses) | Apache-2.0 |
| [`google.golang.org/genproto/googleapis/rpc`](https://pkg.go.dev/google.golang.org/genproto/googleapis/rpc?tab=licenses) | Apache-2.0 |
| [`google.golang.org/grpc`](https://pkg.go.dev/google.golang.org/grpc?tab=licenses) | Apache-2.0 |
| [`google.golang.org/protobuf`](https://pkg.go.dev/google.golang.org/protobuf?tab=licenses) | BSD-3-Clause |
<!-- END go-licenses -->

### Container base image

| Component | License |
|---|---|
| [distroless `static-debian13`](https://github.com/GoogleContainerTools/distroless) | [Apache-2.0](https://github.com/GoogleContainerTools/distroless/blob/main/LICENSE) |

The base image contains the Debian packages `base-files`, `ca-certificates`,
`media-types`, `netbase`, `tzdata` and `tzdata-legacy`. Their copyright and
license notices ship in the image under `/usr/share/doc/<package>/copyright`.

## Used but not distributed

ollametry talks to, is built with, or is tested by these. None of them is part
of the binary or the image.

### Services ollametry works with

| Project | License | Role |
|---|---|---|
| [Ollama](https://github.com/ollama/ollama) | [MIT](https://github.com/ollama/ollama/blob/main/LICENSE) | The server ollametry proxies |
| [grafana/otel-lgtm](https://github.com/grafana/docker-otel-lgtm) | [Apache-2.0](https://github.com/grafana/docker-otel-lgtm/blob/main/LICENSE) | OpenTelemetry backend that receives the metrics |
| [Prometheus](https://github.com/prometheus/prometheus) | [Apache-2.0](https://github.com/prometheus/prometheus/blob/main/LICENSE) | Metric storage inside otel-lgtm |
| [Grafana](https://github.com/grafana/grafana) | [AGPL-3.0](https://github.com/grafana/grafana/blob/main/LICENSE) | Renders the dashboard; the dashboard JSON in this repository is ollametry's own work |

### Specifications

| Specification | License | Role |
|---|---|---|
| [OpenTelemetry semantic conventions](https://github.com/open-telemetry/semantic-conventions) | [Apache-2.0](https://github.com/open-telemetry/semantic-conventions/blob/main/LICENSE) | Metric and attribute names (`gen_ai.*`, `http.server.*`) |

### Build and development tools

These run in their own containers or processes and are not linked into
ollametry.

| Tool | License | Role |
|---|---|---|
| [Go toolchain](https://github.com/golang/go) and the [`golang` image](https://github.com/docker-library/golang) | [BSD-3-Clause](https://go.dev/LICENSE) | Build and tests |
| [golangci-lint](https://github.com/golangci/golangci-lint) | [GPL-3.0](https://github.com/golangci/golangci-lint/blob/main/LICENSE) | Linting |
| [govulncheck](https://github.com/golang/vuln) | [BSD-3-Clause](https://github.com/golang/vuln/blob/master/LICENSE) | Vulnerability scanning |
| [go-licenses](https://github.com/google/go-licenses) | [Apache-2.0](https://github.com/google/go-licenses/blob/master/LICENSE) | Generating this file |
| [actionlint](https://github.com/rhysd/actionlint) | [MIT](https://github.com/rhysd/actionlint/blob/main/LICENSE.txt) | Workflow linting |
| [zizmor](https://github.com/zizmorcore/zizmor) | [MIT](https://github.com/zizmorcore/zizmor/blob/main/LICENSE) | Workflow security analysis |
| [jq](https://github.com/jqlang/jq) | [MIT](https://github.com/jqlang/jq/blob/master/COPYING) (documentation CC BY 3.0) | Dashboard validation |
| [Docker Compose](https://github.com/docker/compose) | [Apache-2.0](https://github.com/docker/compose/blob/main/LICENSE) | Running the service |

### GitHub Actions

| Action | License |
|---|---|
| [actions/checkout](https://github.com/actions/checkout), [actions/cache](https://github.com/actions/cache), [actions/upload-artifact](https://github.com/actions/upload-artifact), [actions/dependency-review-action](https://github.com/actions/dependency-review-action), [actions/attest-build-provenance](https://github.com/actions/attest-build-provenance) | MIT |
| [github/codeql-action](https://github.com/github/codeql-action) | MIT |
| [docker/setup-buildx-action](https://github.com/docker/setup-buildx-action), [docker/login-action](https://github.com/docker/login-action), [docker/metadata-action](https://github.com/docker/metadata-action), [docker/build-push-action](https://github.com/docker/build-push-action) | Apache-2.0 |
| [ossf/scorecard-action](https://github.com/ossf/scorecard-action) | Apache-2.0 |
