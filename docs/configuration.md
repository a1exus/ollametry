# Configuration

[← README](../README.md)

## ollametry

Flags take precedence over `OLLAMETRY_*` variables; Compose sets the
variables. `docker run --rm ollametry:dev -help` lists the flags.

| Variable | Default (binary) | Compose default | Purpose |
|---|---|---|---|
| `OLLAMETRY_LISTEN_ADDR` | `127.0.0.1:11435` | `0.0.0.0:11435` (image) | Proxy listener |
| `OLLAMETRY_ADMIN_ADDR` | `127.0.0.1:11436` | `0.0.0.0:11436` (image) | `/livez`, `/readyz` |
| `OLLAMETRY_OLLAMA_URL` | `http://127.0.0.1:11434` | `http://host.docker.internal:11434` | Upstream Ollama |
| `OLLAMETRY_POLL_INTERVAL` | `15s` | `15s` | `/api/ps` and `/api/tags` polling |
| `OLLAMETRY_SHUTDOWN_TIMEOUT` | `30s` | `30s` | Grace period for in-flight streams |
| `OLLAMETRY_MAX_INSPECT_BYTES` | `33554432` | same | Largest request body / response record decoded; larger ones pass through unmeasured |
| `OLLAMETRY_CARDINALITY_LIMIT` | `2000` | same | Attribute sets per metric |
| `OLLAMETRY_LOG_LEVEL`, `OLLAMETRY_LOG_FORMAT` | `info`, `json` | same | Logging (`text` for humans) |

## OpenTelemetry exporter

The exporter is configured by the standard OpenTelemetry variables:

| Variable | Compose default | Purpose |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://otel-lgtm:4318` | OTLP/HTTP receiver |
| `OTEL_METRIC_EXPORT_INTERVAL` | `15000` | Export interval in ms |
| `OTEL_RESOURCE_ATTRIBUTES` | `deployment.environment.name=local` | Extra resource attributes |
| `OTEL_SERVICE_NAME` | `ollametry` | `service.name` |
| `OTEL_METRICS_EXPORTER` | `otlp` | `prometheus` serves a scrape endpoint instead; `console` prints; `none` disables |

## Compose

Set these in the shell or in a `.env` file next to `compose.yaml` (not
committed):

| Variable | Default | Purpose |
|---|---|---|
| `BIND_ADDRESS` | `127.0.0.1` | Host address the ports bind to |
| `PROXY_PORT` | `11435` | Host port for the proxy |
| `ADMIN_PORT` | `11436` | Host port for `/livez` and `/readyz` |
| `OLLAMA_UPSTREAM` | `http://host.docker.internal:11434` | Ollama as seen from the container |
| `GRAFANA_NETWORK` | `grafana_default` | External network shared with `otel-lgtm` |
| `OLLAMETRY_MEM_LIMIT` | `256m` | Container memory limit |

ollametry has no authentication. Keep `BIND_ADDRESS` on loopback unless a
protected front end sits in front of it (see [SECURITY.md](../SECURITY.md)).
