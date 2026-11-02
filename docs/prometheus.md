# Prometheus exporter

[← README](../README.md)

- [The /metrics endpoint](#the-metrics-endpoint)
- [Scraping it](#scraping-it)
- [Running without otel-lgtm](#running-without-otel-lgtm)
- [Same series as OTLP](#same-series-as-otlp)
- [Turning it off](#turning-it-off)

Ollama has no metrics endpoint. ollametry gives it one: besides pushing OTLP
to otel-lgtm, it serves every metric it records in the Prometheus text format,
so any Prometheus, Grafana Agent/Alloy, VictoriaMetrics or other scraper can
collect Ollama metrics directly.

```
 Prometheus ──scrape──► ollametry :11436/metrics
                        ollametry ──OTLP push──► otel-lgtm :4318   (optional)
```

## The /metrics endpoint

| | |
|---|---|
| URL | `http://127.0.0.1:11436/metrics` (the admin listener, next to `/livez` and `/readyz`) |
| Format | Prometheus text exposition |
| Content | Every metric in the [metric reference](metrics.md#metric-reference), Go runtime metrics, and `target_info` with the resource (`service_name`, `service_version`, ...) |
| Default | On (`OLLAMETRY_PROMETHEUS=true`) |

```sh
curl -s http://127.0.0.1:11436/metrics | grep -E '^(ollama_up|gen_ai_client_token_usage_sum)'
```

The endpoint reads the same instruments, views and cardinality limit as the
OTLP exporter, so both always agree. Its output passes `promtool check
metrics` except for one naming lint on `go_goroutine_count`, a name set by the
OpenTelemetry Go runtime conventions.

## Scraping it

```yaml
scrape_configs:
  - job_name: ollametry
    scrape_interval: 15s
    static_configs:
      - targets: ["127.0.0.1:11436"]   # ollametry:11436 from a container on the same network
```

The admin port binds to `127.0.0.1` by default. To scrape from another host,
set `BIND_ADDRESS` (see [configuration](configuration.md#compose)) and put
something in front that restricts access: the metrics contain model names, and
ollametry has no authentication.

## Running without otel-lgtm

To use ollametry purely as an Ollama exporter, stop the OTLP push and drop the
`grafana_default` network with a `compose.override.yaml` next to
`compose.yaml` (Compose reads it automatically):

```yaml
services:
  ollametry:
    networks: !reset [default]
    environment:
      OTEL_METRICS_EXPORTER: none
```

Start it with `docker compose up -d --build` (`make up` checks for the Grafana
network first). Outside Docker, the binary alone does the same:

```sh
OTEL_METRICS_EXPORTER=none ollametry -ollama-url http://127.0.0.1:11434
```

## Same series as OTLP

The names and labels match what the OTLP path produces in otel-lgtm's
Prometheus, so the bundled [dashboard](../deploy/grafana/ollametry.json) and
the queries in [metrics](metrics.md#querying-in-prometheus) work against
either source.

| | OTLP via otel-lgtm | Scraped `/metrics` |
|---|---|---|
| Metric names and labels | `gen_ai_*`, `ollama_*`, `http_server_*` | identical |
| Resource | `job` = `service_name`, plus promoted resource labels | `target_info` series; `job` and `instance` from the scrape config |
| Timing | Pushed every `OTEL_METRIC_EXPORT_INTERVAL` | Read at every scrape |
| Stale series | Linger 5 minutes (no staleness markers) | Marked stale on the next scrape |

## Turning it off

```sh
OLLAMETRY_PROMETHEUS=false make up
```

`/metrics` then answers 404; OTLP export is unaffected.
