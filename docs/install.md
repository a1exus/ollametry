# Installing

[← README](../README.md)

- [Choosing a way to run it](#choosing-a-way-to-run-it)
- [Binary](#binary)
- [macOS service (launchd)](#macos-service-launchd)
- [Container image](#container-image)
- [From source](#from-source)

## Choosing a way to run it

| Way | Fits when |
|---|---|
| [Docker Compose](operations.md#running-with-docker-compose) from this repository | You run the `~/opt/grafana` stack; `make up` builds and starts it |
| [Binary](#binary) as a [launchd service](#macos-service-launchd) | Ollama runs natively on a Mac and you want ollametry next to it, without Docker |
| [Container image](#container-image) | You run containers elsewhere and want a published image |

Every way serves the same proxy (`:11435`), health and `/metrics`
(`:11436`), and OTLP push (see [configuration](configuration.md)).

## Binary

Each [release](https://github.com/a1exus/ollametry/releases) carries archives
for macOS and Linux (`arm64`, `amd64`) and a `SHA256SUMS` file. Archives
contain the binary, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `README.md`, and on
macOS the launchd agent.

```sh
version=0.1.0 os=darwin arch=arm64     # linux and/or amd64 as needed
base=https://github.com/a1exus/ollametry/releases/download/v$version
curl -fsSLO "$base/ollametry_${version}_${os}_${arch}.tar.gz"
curl -fsSLO "$base/SHA256SUMS"
```

Verify before running it. The checksum proves the download is intact; the
attestation proves the archive was built by this repository's release workflow
from the tagged commit:

```sh
shasum -a 256 --check --ignore-missing SHA256SUMS      # sha256sum on Linux
gh attestation verify "ollametry_${version}_${os}_${arch}.tar.gz" -R a1exus/ollametry
```

Install and check:

```sh
tar -xzf "ollametry_${version}_${os}_${arch}.tar.gz"
sudo install -m 0755 "ollametry_${version}_${os}_${arch}/ollametry" /usr/local/bin/
ollametry version
```

The macOS binaries are not signed or notarized by Apple. Files downloaded
with `curl` run as shown; a file downloaded with a browser is quarantined by
Gatekeeper and needs `xattr -d com.apple.quarantine /usr/local/bin/ollametry`
after you have verified it.

The archives are reproducible: `make dist VERSION=vX.Y.Z` on the tagged
commit rebuilds them with the same checksums.

## macOS service (launchd)

[`deploy/launchd/io.github.a1exus.ollametry.plist`](../deploy/launchd/io.github.a1exus.ollametry.plist)
(also in the macOS archive) runs ollametry as a per-user agent: it starts at
login, restarts after a crash, and stops cleanly on `bootout`. Review its
`EnvironmentVariables` first: by default it pushes OTLP to
`http://127.0.0.1:4318`; set `OTEL_METRICS_EXPORTER` to `none` to only serve
`/metrics`.

```sh
cp io.github.a1exus.ollametry.plist ~/Library/LaunchAgents/
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/io.github.a1exus.ollametry.plist
curl -s http://127.0.0.1:11436/readyz                 # ok once Ollama answers
```

| Task | Command |
|---|---|
| Status | `launchctl print "gui/$(id -u)/io.github.a1exus.ollametry"` |
| Logs | `tail -f /tmp/ollametry.log` |
| Restart | `launchctl kickstart -k "gui/$(id -u)/io.github.a1exus.ollametry"` |
| Stop and unload | `launchctl bootout "gui/$(id -u)/io.github.a1exus.ollametry"` |

Do not run the agent and `make up` at the same time: both listen on
`127.0.0.1:11435` and `:11436`.

## Container image

```sh
docker run -d --name ollametry -p 127.0.0.1:11435:11435 -p 127.0.0.1:11436:11436 \
  -e OLLAMETRY_OLLAMA_URL=http://host.docker.internal:11434 \
  -e OTEL_METRICS_EXPORTER=none \
  ghcr.io/a1exus/ollametry:0.1.0
```

The image is distroless, runs as a non-root user, and carries `LICENSE` and
`THIRD_PARTY_NOTICES.md` in `/usr/share/doc/ollametry/`. Verify it with
`gh attestation verify oci://ghcr.io/a1exus/ollametry:0.1.0 -R a1exus/ollametry`.
To run it from this repository's Compose file instead of building, see
[releases](releases.md#using-a-released-image).

## From source

```sh
go install github.com/a1exus/ollametry/cmd/ollametry@latest
ollametry version
```

`go install` needs the Go version in [`go.mod`](../go.mod). The binary reports
the module version it was installed at.
