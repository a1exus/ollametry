# Container environments

[← README](../README.md)

- [What the container needs](#what-the-container-needs)
- [Linux: Docker Engine](#linux-docker-engine)
- [macOS: Docker Desktop](#macos-docker-desktop)
- [macOS: Apple container](#macos-apple-container)
- [Windows: WSL 2](#windows-wsl-2)

| OS | Runtime | Compose and `make up` | Status |
|---|---|---|---|
| Linux | [Docker Engine](https://docs.docker.com/engine/install/) | Yes, with one override | Image build and smoke test run on Docker Engine in CI |
| macOS | Docker Desktop | Yes, as is | Verified: the project is developed and E2E-tested here |
| macOS | [Apple `container`](https://github.com/apple/container) | No Compose; `container run` | Not tested by this project |
| Windows | [WSL 2](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-containers) with Docker Desktop or Docker Engine | Yes | Not tested by this project |

On a Mac, the [native binary as a launchd service](install.md#macos-service-launchd)
needs no container runtime at all.

## What the container needs

ollametry in a container has to reach Ollama on the host. Ollama listens on
`127.0.0.1:11434` by default, and a container's own `127.0.0.1` is the
container, not the host. Each runtime bridges that differently:

| Ollama listens on | Host header Ollama accepts |
|---|---|
| Loopback (default) | `localhost`, any `*.localhost`, `*.local` or `*.internal` name, loopback or private IPs, the machine's own addresses |
| Any other address (`OLLAMA_HOST=0.0.0.0`) | Anything |

ollametry sends the upstream URL's host, so `host.docker.internal` and
`host.container.internal` pass the check. If you make Ollama listen on all
interfaces, it is reachable from your network too, and it has no
authentication: restrict it with a firewall.

The Makefile's development targets (`make test`, `make lint`, ...) run tools
with the `docker` CLI and work on any Docker Engine or Docker Desktop.

## Linux: Docker Engine

Install [Docker Engine](https://docs.docker.com/engine/install/) and the
Compose plugin (v2.24 or newer). On Linux, `host.docker.internal` does not
exist unless mapped to `host-gateway`, the Docker bridge address (usually
`172.17.0.1`), so Ollama must listen there as well as on loopback:

1. Make Ollama listen on all interfaces. With the official installer's
   systemd service:

   ```sh
   sudo systemctl edit ollama
   #   [Service]
   #   Environment="OLLAMA_HOST=0.0.0.0:11434"
   sudo systemctl restart ollama
   ```

   Then block port 11434 from other machines with your firewall.

2. Map `host.docker.internal` in a `compose.override.yaml` next to
   `compose.yaml`:

   ```yaml
   services:
     ollametry:
       extra_hosts:
         - host.docker.internal:host-gateway
   ```

3. `make up && make health`.

Without Docker, run the [Linux binary](install.md#binary) directly; it reaches
Ollama on `127.0.0.1` with no changes.

## macOS: Docker Desktop

The default: `make up` works as is. Docker Desktop resolves
`host.docker.internal` to the Mac and forwards to its loopback, where the
Ollama app listens. See [operations](operations.md#running-with-docker-compose).

## macOS: Apple container

Apple's [`container`](https://github.com/apple/container) runs Linux
containers in lightweight VMs on Apple silicon. It has no Compose and no
`host.docker.internal`, so run ollametry with `container run`. This project
does not test it; the commands follow Apple's
[host integration](https://github.com/apple/container/blob/main/docs/host-integration.md)
and [networking](https://github.com/apple/container/blob/main/docs/networking.md)
guides.

1. Start the service and give the Mac a name containers can reach. The IP is
   any unused address (here from the 203.0.113.0/24 documentation range); the
   name ends in `.internal`, which Ollama accepts:

   ```sh
   container system start
   sudo container system dns create host.container.internal --localhost 203.0.113.113
   ```

   The packet filter rule behind this is removed on restart; run the `dns
   create` command again after rebooting. Creating it turns off iCloud
   Private Relay.

2. Run a [released image](install.md#container-image):

   ```sh
   container run -d --name ollametry \
     -p 127.0.0.1:11435:11435 -p 127.0.0.1:11436:11436 \
     -e OLLAMETRY_OLLAMA_URL=http://host.container.internal:11434 \
     -e OTEL_METRICS_EXPORTER=none \
     ghcr.io/a1exus/ollametry:X.Y.Z
   curl -s http://127.0.0.1:11436/readyz
   ```

   To push OTLP to otel-lgtm running under Docker Desktop, replace
   `OTEL_METRICS_EXPORTER=none` with
   `OTEL_EXPORTER_OTLP_ENDPOINT=http://host.container.internal:4318`: the
   two runtimes share no network, only the Mac's published ports.

## Windows: WSL 2

Containers on Windows run inside WSL 2. Follow Microsoft's
[WSL containers tutorial](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-containers),
then pick the case that matches where Ollama runs:

| Ollama runs in | Docker runs as | Setup |
|---|---|---|
| Windows (the Ollama app) | Docker Desktop with the WSL 2 backend | `make up` from a WSL shell; Docker Desktop provides `host.docker.internal`. If ollametry reports Ollama unreachable, set the Windows environment variable `OLLAMA_HOST=0.0.0.0:11434`, restart Ollama, and restrict port 11434 in Windows Firewall |
| Windows | Docker Engine inside a WSL distribution | Set `OLLAMA_HOST=0.0.0.0:11434` on Windows, and point ollametry at the Windows host from WSL: with the default NAT networking that is the WSL default gateway (`ip route show default`); set `OLLAMA_UPSTREAM=http://<gateway>:11434` |
| The WSL distribution (Linux install) | Docker Engine in the same distribution | As [Linux](#linux-docker-engine) |

Run `make` from the WSL shell, with the repository on the Linux file system
(`~/...`, not `/mnt/c/...`), which is much faster for bind mounts.
