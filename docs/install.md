# Installing

[← README](../README.md)

- [How ollametry runs](#how-ollametry-runs)
- [Building from source](#building-from-source)
- [Using a published image](#using-a-published-image)
- [Verifying an image](#verifying-an-image)

## How ollametry runs

ollametry runs in a container with Docker Compose, through `make`, next to
Ollama on the host. Ollama stays on the host everywhere except native Linux:
on macOS and Windows, containers run inside a VM, away from the GPU
([GPU access](containers.md#where-ollama-runs-gpu-access)).

| OS | Docker |
| --- | --- |
| macOS | [Docker Desktop](containers.md#macos-docker-desktop) |
| Linux | [Docker Engine](containers.md#linux-docker-engine) with the Compose plugin; on native Linux, Ollama itself may also run in a container with an [NVIDIA](containers.md#nvidia-gpus-on-native-linux) or AMD GPU |
| Windows | [WSL 2](containers.md#windows-wsl-2) with Docker Desktop or Docker Engine |

You need Docker Compose v2.24 or newer, `make`, `curl` and `bash` (on
Windows, in the WSL shell). The image is `linux/amd64` and `linux/arm64`,
distroless, and runs as a non-root user. It serves the proxy (`:11435`),
health and `/metrics` (`:11436`), and OTLP push (see
[configuration](configuration.md)).

## Building from source

`make up` builds the image from this repository and starts it next to the
`~/opt/grafana` stack:

```sh
make up        # docker compose up -d --build, wait until healthy
make health    # Ollama, the proxy, readiness
```

Lifecycle, health and troubleshooting are in [operations](operations.md).

## Using a published image

Each release publishes `ghcr.io/a1exus/ollametry:<version>` (also `X.Y`, and
`X` from 1.0). To run it instead of building, add a `compose.override.yaml`
next to `compose.yaml`; Compose reads it automatically, and `!reset` removes
the build section:

```yaml
services:
  ollametry:
    build: !reset null
    image: ghcr.io/a1exus/ollametry:0.1.0
    pull_policy: missing
```

`make up` then pulls the image instead of building it; everything else stays
the same.

## Verifying an image

Release images carry an SBOM and signed build provenance. Verify that an image
was built by this repository's release workflow from the tagged commit:

```sh
gh attestation verify oci://ghcr.io/a1exus/ollametry:0.1.0 -R a1exus/ollametry
```

The image carries `LICENSE` and `THIRD_PARTY_NOTICES.md` in
`/usr/share/doc/ollametry/`, and `docker compose exec ollametry ollametry
version` prints its version.
