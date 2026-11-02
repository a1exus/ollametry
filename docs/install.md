# Installing

[← README](../README.md)

- [How ollametry runs](#how-ollametry-runs)
- [With this repository (Docker Compose)](#with-this-repository-docker-compose)
- [From the published image](#from-the-published-image)
- [Verifying an image](#verifying-an-image)

## How ollametry runs

ollametry runs in a container, next to Ollama on the host. Ollama stays on
the host because containers get no GPU on macOS, and none on Windows without
an NVIDIA GPU ([GPU access](containers.md#where-ollama-should-run-gpu-access)).
Any container runtime works; [container environments](containers.md) covers
how each one reaches Ollama on the host:

| OS | Container runtime |
| --- | --- |
| macOS | [Docker Desktop](containers.md#macos-docker-desktop), [Apple `container`](containers.md#macos-apple-container) |
| Linux | [Docker Engine](containers.md#linux-docker-engine), with [NVIDIA GPUs](containers.md#nvidia-gpus-on-linux-and-wsl-2) for Ollama itself if you want |
| Windows | [WSL 2](containers.md#windows-wsl-2) with Docker Desktop or Docker Engine |

The image is `linux/amd64` and `linux/arm64`, distroless, and runs as a
non-root user. Every way serves the same proxy (`:11435`), health and
`/metrics` (`:11436`), and OTLP push (see [configuration](configuration.md)).

## With this repository (Docker Compose)

This repository's setup builds the image from source and runs it next to the
`~/opt/grafana` stack:

```sh
make up        # docker compose up -d --build, wait until healthy
make health    # Ollama, the proxy, readiness
```

Lifecycle, health and troubleshooting are in [operations](operations.md). To
run a published image with the same Compose file instead of building one, see
[releases](releases.md#using-a-released-image).

## From the published image

Each release publishes `ghcr.io/a1exus/ollametry:<version>` (also `X.Y`, and
`X` from 1.0):

```sh
docker run -d --name ollametry --restart unless-stopped \
  -p 127.0.0.1:11435:11435 -p 127.0.0.1:11436:11436 \
  -e OLLAMETRY_OLLAMA_URL=http://host.docker.internal:11434 \
  -e OTEL_METRICS_EXPORTER=none \
  ghcr.io/a1exus/ollametry:0.1.0
curl -s http://127.0.0.1:11436/readyz      # ok once ollametry reaches Ollama
docker exec ollametry ollametry version
```

`OTEL_METRICS_EXPORTER=none` serves `/metrics` for Prometheus only; set
`OTEL_EXPORTER_OTLP_ENDPOINT` instead to push OTLP. On Linux,
`host.docker.internal` needs
[one more setting](containers.md#linux-docker-engine).

## Verifying an image

Release images carry an SBOM and signed build provenance. Verify that an image
was built by this repository's release workflow from the tagged commit:

```sh
gh attestation verify oci://ghcr.io/a1exus/ollametry:0.1.0 -R a1exus/ollametry
```

The image carries `LICENSE` and `THIRD_PARTY_NOTICES.md` in
`/usr/share/doc/ollametry/`.
