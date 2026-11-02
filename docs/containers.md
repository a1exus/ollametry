# Container environments

[← README](../README.md)

- [Where Ollama runs: GPU access](#where-ollama-runs-gpu-access)
- [What the container needs](#what-the-container-needs)
- [Linux: Docker Engine](#linux-docker-engine)
- [macOS: Docker Desktop](#macos-docker-desktop)
- [macOS: installing with Homebrew](#macos-installing-with-homebrew)
- [Windows: WSL 2](#windows-wsl-2)
- [NVIDIA GPUs on native Linux](#nvidia-gpus-on-native-linux)

ollametry runs with Docker Compose (`make up`), so every platform needs Docker
with Compose v2.24 or newer:

| OS | Docker | Status |
| --- | --- | --- |
| Linux | [Docker Engine](https://docs.docker.com/engine/install/) with the Compose plugin, plus one override | Image build and smoke test run on Docker Engine in CI |
| macOS | Docker Desktop, as is | Verified: the project is developed and E2E-tested here |
| Windows | [WSL 2](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-containers) with Docker Desktop or Docker Engine | Not tested by this project |

## Where Ollama runs: GPU access

ollametry always runs in a container and needs no GPU. Ollama needs one:
without it, it falls back to the CPU, which is many times slower. This
project's rule follows from that:

**Ollama runs on the host. The only exception is native Linux (not inside a
virtual machine), where Ollama may also run in a container with the GPU.**

| Host | Where Ollama runs | Why |
| --- | --- | --- |
| macOS | On the host: the Ollama app or Homebrew | Containers run in a Linux VM without GPU passthrough: "GPU acceleration is not available for Docker Desktop in macOS due to the lack of GPU passthrough and emulation" ([Ollama FAQ](https://docs.ollama.com/faq#how-do-i-use-ollama-with-gpu-acceleration-in-docker)). |
| Windows | On the host: the Ollama app | Docker Desktop and Docker Engine on Windows run containers inside the WSL 2 virtual machine, like macOS. Docker Desktop can pass an NVIDIA GPU into it ([Docker GPU support](https://docs.docker.com/desktop/features/gpu/)), but the project keeps Ollama on the host, with direct access to the GPU and its drivers, on every OS that puts containers in a VM |
| Linux, native | On the host, or in a container with the GPU: NVIDIA through the [NVIDIA Container Toolkit](#nvidia-gpus-on-native-linux), AMD through the `ollama/ollama:rocm` image ([Ollama Docker docs](https://docs.ollama.com/docker)) | Containers share the host's kernel and GPU drivers; no VM in between |
| Linux in a VM, including WSL 2 distributions | On the machine that has the GPU | Same reason as macOS and Windows |

## What the container needs

ollametry in a container has to reach Ollama on the host. Ollama listens on
`127.0.0.1:11434` by default, and a container's own `127.0.0.1` is the
container, not the host. Each runtime bridges that differently:

| Ollama listens on | Host header Ollama accepts |
| --- | --- |
| Loopback (default) | `localhost`, any `*.localhost`, `*.local` or `*.internal` name, loopback or private IPs, the machine's own addresses |
| Any other address (`OLLAMA_HOST=0.0.0.0`) | Anything |

ollametry sends the upstream URL's host, so `host.docker.internal` passes the
check. If you make Ollama listen on all
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

## macOS: Docker Desktop

The default: `make up` works as is. Docker Desktop resolves
`host.docker.internal` to the Mac and forwards to its loopback, where the
Ollama app listens. See [operations](operations.md#running-with-docker-compose).

## macOS: installing with Homebrew

Everything on the Mac side installs with [Homebrew](https://brew.sh):

```sh
brew install --cask ollama-app       # the Ollama app (menu bar), as used here
brew install --cask docker-desktop   # Docker Desktop, for make up and the toolchain
```

To run Ollama as a background service without the app, use the formula
instead of the cask: `brew install ollama && brew services start ollama`.
Either way Ollama runs on the host with the GPU, which is
[where it should run](#where-ollama-runs-gpu-access) on a Mac.

## Windows: WSL 2

Containers on Windows run inside WSL 2. Follow Microsoft's
[WSL containers tutorial](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-containers),
then pick the case that matches your Docker. Ollama runs on Windows in both
([why](#where-ollama-runs-gpu-access)), never in a container or a WSL
distribution:

| Docker runs as | Setup |
| --- | --- |
| Docker Desktop with the WSL 2 backend | `make up` from a WSL shell; Docker Desktop provides `host.docker.internal`. If ollametry reports Ollama unreachable, set the Windows environment variable `OLLAMA_HOST=0.0.0.0:11434`, restart Ollama, and restrict port 11434 in Windows Firewall |
| Docker Engine inside a WSL distribution | Set `OLLAMA_HOST=0.0.0.0:11434` on Windows, and point ollametry at the Windows host from WSL: with the default NAT networking that is the WSL default gateway (`ip route show default`); set `OLLAMA_UPSTREAM=http://<gateway>:11434` |

Run `make` from the WSL shell, with the repository on the Linux file system
(`~/...`, not `/mnt/c/...`), which is much faster for bind mounts.

## NVIDIA GPUs on native Linux

On native Linux (not in a VM), Ollama may run in a container with the GPU
([the rule](#where-ollama-runs-gpu-access)). With an NVIDIA GPU, Docker needs the
[NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html).
This project does not test GPU containers; the commands below are NVIDIA's
and Ollama's.

**Driver.** Install the NVIDIA driver from your distribution.

**Toolkit** (Ubuntu or Debian), from NVIDIA's install guide:

```sh
sudo apt-get update && sudo apt-get install -y --no-install-recommends ca-certificates curl gnupg2
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor \
  -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
  && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
    sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt-get update && sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
```

NVIDIA's guide also shows pinning the package version and the `yum`/`dnf`
equivalents.

**Verify** (NVIDIA's
[sample workload](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/sample-workload.html)):

```sh
sudo docker run --rm --runtime=nvidia --gpus all ubuntu nvidia-smi
```

**Run Ollama** with the GPU, on the network `make up` creates (`ollama_default`,
from this repository's Compose project), so ollametry reaches it by name:

```sh
OLLAMA_UPSTREAM=http://ollama-gpu:11434 make up
docker run -d --gpus=all --network ollama_default --name ollama-gpu \
  -v ollama:/root/.ollama ollama/ollama
curl -s http://127.0.0.1:11436/readyz      # ok once ollametry reaches Ollama
```

The Ollama image listens on all interfaces inside the container, so the Host
check does not apply, and nothing is published to the host: clients go through
ollametry on `127.0.0.1:11435`. Add `-p 127.0.0.1:11434:11434` if you also want
direct access. For AMD GPUs on Linux, use `--device /dev/kfd --device /dev/dri`
and `ollama/ollama:rocm` instead of `--gpus=all` and `ollama/ollama`.
