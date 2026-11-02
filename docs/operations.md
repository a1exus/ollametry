# Operations

[← README](../README.md)

- [Running with Docker Compose](#running-with-docker-compose)
- [Endpoints and health](#endpoints-and-health)
- [Grafana dashboard](#grafana-dashboard)
- [Upgrading](#upgrading)
- [Troubleshooting](#troubleshooting)

## Running with Docker Compose

This page covers this repository's Docker Compose setup. Running the binary
natively as a launchd, systemd or Task Scheduler service is covered in
[install](install.md).

ollametry runs as one Compose service, `ollametry`, defined in
[`compose.yaml`](../compose.yaml). It joins the `grafana_default` network of
`~/opt/grafana` to reach `otel-lgtm`, so start that stack first. Every
lifecycle target is a thin wrapper around `docker compose`:

| Target | Runs | Use it to |
|---|---|---|
| `make up` | `docker compose up -d --build --wait` | Build if needed, start, wait until healthy |
| `make down` | `docker compose down --remove-orphans` | Stop and remove the container and network |
| `make start` / `make stop` | `docker compose start` / `stop` | Pause and resume without removing the container |
| `make restart` | `docker compose restart` | Restart; does not rebuild (use `make up` after code changes) |
| `make build` | `docker compose build --pull` | Build the image |
| `make rebuild` | `docker compose build --pull --no-cache` | Build from scratch |
| `make clean` | `docker compose down --rmi local` | Remove the container, network and built image |
| `make ps` | `docker compose ps --all` | Container status, stopped ones included |
| `make logs` | `docker compose logs --follow` | Follow logs (`TAIL=200` lines of history) |
| `make top` | `docker compose top` | Processes in the container |
| `make images` | `docker compose images` | Images used by the project |
| `make validate` | `docker compose config --quiet` | Check the config and that the Grafana network exists |
| `make config` | `docker compose config` | Print the resolved config |

The container is read-only, runs as a non-root user with all capabilities
dropped and `no-new-privileges`, is limited to 256 MiB and 256 processes, and
binds its ports to `127.0.0.1` only. Logs are JSON, rotated at 3 × 10 MB.

## Endpoints and health

The admin endpoints are specified in [`api/openapi.yaml`](../api/openapi.yaml)
(OpenAPI 3.1); a contract test keeps it in step with the handlers.

| Endpoint | URL |
|---|---|
| Proxy (use instead of Ollama) | `http://127.0.0.1:11435` |
| Liveness | `http://127.0.0.1:11436/livez`: the process serves requests |
| Readiness | `http://127.0.0.1:11436/readyz`: the last poll reached Ollama within three intervals |
| Prometheus metrics | `http://127.0.0.1:11436/metrics` ([Prometheus exporter](prometheus.md)) |
| Dashboard (folder "ollama") | `https://grafana.localhost/d/ollametry` |

`make health` checks Ollama, the proxy and readiness in one go; `make smoke
MODEL=<name>` sends one streamed chat request through the proxy. The image's
own healthcheck (`ollametry healthcheck`, as distroless has no curl) is what
`make up` waits for.

On `SIGTERM` (`make down`, `make stop`) ollametry stops accepting requests,
gives in-flight streams up to `OLLAMETRY_SHUTDOWN_TIMEOUT` (30 s) to finish,
records their metrics, and flushes a final export. Compose waits 40 s before
killing it.

## Grafana dashboard

The dashboard lives in [`deploy/grafana/ollametry.json`](../deploy/grafana/ollametry.json)
and is provisioned from files by `~/opt/grafana`:

```sh
make -C ~/opt/grafana dashboards
```

Grafana refuses UI and API saves of provisioned dashboards, so edit the JSON
here, run `make dashboard-check`, and sync again.

## Upgrading

```sh
git pull
make up     # rebuilds the image and replaces the container
```

Series from the old container stay visible in Prometheus for 5 minutes with
the previous `service_version`; the dashboard aggregates by model, so panels
are not affected.

## Troubleshooting

**`make up` fails with "docker network grafana_default not found".**
The Grafana stack is not running. Run `make up` in `~/opt/grafana` first, or
set `GRAFANA_NETWORK` if it uses another network.

**`/readyz` fails, `ollama_up` is 0.**
ollametry cannot reach Ollama. Check that Ollama runs (`curl
127.0.0.1:11434/api/version`) and, if you moved it, that `OLLAMA_UPSTREAM`
points at the new port. `make logs` shows "ollama unreachable" with the error.

**No metrics in Grafana.**
Exports go to `otel-lgtm:4318` every 15 s. Check them without Grafana:

```sh
docker exec otel-lgtm curl -s -G localhost:9090/api/v1/query --data-urlencode 'query=ollama_up'
```

Empty means the exporter cannot reach `otel-lgtm`: check `make validate` and
that `otel-lgtm` is on the same network.

**Requests show up but without token counts.**
Either the client calls Ollama directly on `:11434` (see
[clients](clients.md)), or it uses OpenAI-compatible streaming without
`stream_options.include_usage`.

**A model's first request is missing from rates.**
Expected: an OTLP counter's first export is its baseline in Prometheus.

**`gen_ai.request.model` is `_OTHER`.**
The requested name is not installed or loaded (often a typo or a missing
tag), or the model was pulled after the last poll; it resolves within one
poll interval. The log line shows the name as sent.
