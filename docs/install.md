# Installing

[← README](../README.md)

- [Choosing a way to run it](#choosing-a-way-to-run-it)
- [Binary](#binary)
- [macOS service (launchd)](#macos-service-launchd)
- [Linux service (systemd)](#linux-service-systemd)
- [Windows (Task Scheduler)](#windows-task-scheduler)
- [Container image](#container-image)
- [From source](#from-source)

## Choosing a way to run it

ollametry runs next to Ollama, which belongs on the host on macOS and on
Windows without an NVIDIA GPU (see [GPU access](containers.md#where-ollama-should-run-gpu-access)).
Pick the way that matches your machine:

| OS | Native binary as a service | Container |
| --- | --- | --- |
| macOS (Apple silicon, Intel) | [launchd agent](#macos-service-launchd) | [Docker Desktop or Apple `container`](containers.md) |
| Linux (`amd64`, `arm64`) | [systemd unit](#linux-service-systemd) | [Docker Engine](containers.md#linux-docker-engine) |
| Windows (`amd64`, `arm64`) | [Task Scheduler](#windows-task-scheduler) | [WSL 2](containers.md#windows-wsl-2) |

The native binary needs nothing else installed and reaches Ollama on
`127.0.0.1` directly. This repository's own setup is
[Docker Compose](operations.md#running-with-docker-compose) (`make up`) next to
the `~/opt/grafana` stack.

Every way serves the same proxy (`:11435`), health and `/metrics`
(`:11436`), and OTLP push (see [configuration](configuration.md)).

## Binary

Each [release](https://github.com/a1exus/ollametry/releases) carries one
archive per platform and a `SHA256SUMS` file:

| OS | Archive | Contains |
| --- | --- | --- |
| macOS | `ollametry_<version>_darwin_<arm64\|amd64>.tar.gz` | `ollametry`, the [launchd agent](#macos-service-launchd) |
| Linux | `ollametry_<version>_linux_<amd64\|arm64>.tar.gz` | `ollametry`, the [systemd unit](#linux-service-systemd) |
| Windows | `ollametry_<version>_windows_<amd64\|arm64>.zip` | `ollametry.exe` |

Every archive also contains `LICENSE`, `THIRD_PARTY_NOTICES.md` and
`README.md`. Verify a download before running it: the checksum proves it is
intact, and the attestation proves it was built by this repository's release
workflow from the tagged commit.

**macOS and Linux:**

```sh
version=0.1.0 os=darwin arch=arm64     # os=linux, arch=amd64 as needed
base=https://github.com/a1exus/ollametry/releases/download/v$version
curl -fsSLO "$base/ollametry_${version}_${os}_${arch}.tar.gz"
curl -fsSLO "$base/SHA256SUMS"
shasum -a 256 --check --ignore-missing SHA256SUMS     # sha256sum on Linux
gh attestation verify "ollametry_${version}_${os}_${arch}.tar.gz" -R a1exus/ollametry
tar -xzf "ollametry_${version}_${os}_${arch}.tar.gz"
sudo install -m 0755 "ollametry_${version}_${os}_${arch}/ollametry" /usr/local/bin/
ollametry version
```

**Windows** (PowerShell):

```powershell
$version = "0.1.0"; $arch = "amd64"                  # or arm64
$name = "ollametry_${version}_windows_${arch}"
$base = "https://github.com/a1exus/ollametry/releases/download/v$version"
Invoke-WebRequest "$base/$name.zip" -OutFile "$name.zip"
Invoke-WebRequest "$base/SHA256SUMS" -OutFile SHA256SUMS
$expected = ((Select-String -Path SHA256SUMS -Pattern "$name.zip").Line -split '\s+')[0]
if ((Get-FileHash "$name.zip" -Algorithm SHA256).Hash.ToLower() -ne $expected) { throw "checksum mismatch" }
gh attestation verify "$name.zip" -R a1exus/ollametry
$dir = "$env:LOCALAPPDATA\Programs\ollametry"
Expand-Archive "$name.zip" -DestinationPath $env:TEMP -Force
New-Item -ItemType Directory -Force $dir | Out-Null
Move-Item "$env:TEMP\$name\ollametry.exe" $dir -Force
& "$dir\ollametry.exe" version
```

The binaries are not code-signed: macOS Gatekeeper quarantines a file
downloaded with a browser (`xattr -d com.apple.quarantine /usr/local/bin/ollametry`
after verifying it), and Windows SmartScreen may warn about one
(`Unblock-File` after verifying it). Downloads with `curl` or
`Invoke-WebRequest` run as shown.

The archives are reproducible: `make dist VERSION=vX.Y.Z` on the tagged
commit rebuilds them with the same checksums, on any machine.

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
| --- | --- |
| Status | `launchctl print "gui/$(id -u)/io.github.a1exus.ollametry"` |
| Logs | `tail -f /tmp/ollametry.log` |
| Restart | `launchctl kickstart -k "gui/$(id -u)/io.github.a1exus.ollametry"` |
| Stop and unload | `launchctl bootout "gui/$(id -u)/io.github.a1exus.ollametry"` |

Verified on macOS: the agent starts, serves `/livez`, and stops cleanly.

## Linux service (systemd)

[`deploy/systemd/ollametry.service`](../deploy/systemd/ollametry.service)
(also in the Linux archive) runs ollametry as a system service next to
Ollama's own `ollama.service`. It is hardened: a dynamic user, no
capabilities, a read-only system, and restricted address families and system
calls (`systemd-analyze security` rates it 1.2, OK). By default it only serves
`/metrics`; edit its `Environment=` lines to push OTLP.

```sh
sudo install -m 0644 ollametry.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ollametry
curl -s http://127.0.0.1:11436/readyz                 # ok once Ollama answers
```

| Task | Command |
| --- | --- |
| Status | `systemctl status ollametry` |
| Logs | `journalctl -u ollametry -f` |
| Change settings | `sudo systemctl edit ollametry` (add `Environment=` lines), then restart |
| Restart | `sudo systemctl restart ollametry` |
| Stop and disable | `sudo systemctl disable --now ollametry` |

`systemd-analyze verify` passes on the unit; this project does not run it on
a live Linux host.

## Windows (Task Scheduler)

Windows has no service wrapper for console programs, so run `ollametry.exe` as
a scheduled task that starts at logon, next to the Ollama app. Settings go in
user environment variables; logs go to a file through `cmd.exe`. In
PowerShell, after [installing the binary](#binary):

```powershell
$dir = "$env:LOCALAPPDATA\Programs\ollametry"
[Environment]::SetEnvironmentVariable("OTEL_METRICS_EXPORTER", "none", "User")   # only /metrics
$action = New-ScheduledTaskAction -Execute "cmd.exe" `
  -Argument "/c `"`"$dir\ollametry.exe`" >> `"$dir\ollametry.log`" 2>&1`""
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit ([TimeSpan]::Zero) `
  -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName ollametry -Action $action -Trigger $trigger -Settings $settings
Start-ScheduledTask -TaskName ollametry
Invoke-WebRequest http://127.0.0.1:11436/readyz      # ok once Ollama answers
```

`-ExecutionTimeLimit ([TimeSpan]::Zero)` matters: Task Scheduler otherwise
stops tasks after three days.

| Task | Command |
| --- | --- |
| Status | `Get-ScheduledTask -TaskName ollametry \| Get-ScheduledTaskInfo` |
| Logs | `Get-Content "$env:LOCALAPPDATA\Programs\ollametry\ollametry.log" -Wait` |
| Restart | `Stop-ScheduledTask ollametry; Start-ScheduledTask ollametry` |
| Remove | `Unregister-ScheduledTask -TaskName ollametry -Confirm:$false` |

Stopping the task ends the process without a graceful shutdown, so metrics of
requests in flight at that moment are lost; `Ctrl+C` in a console shuts down
cleanly. The Windows binary passes the unit and integration tests on Windows
in CI; this project does not test the scheduled task.

## Container image

```sh
docker run -d --name ollametry -p 127.0.0.1:11435:11435 -p 127.0.0.1:11436:11436 \
  -e OLLAMETRY_OLLAMA_URL=http://host.docker.internal:11434 \
  -e OTEL_METRICS_EXPORTER=none \
  ghcr.io/a1exus/ollametry:0.1.0
```

Images exist for `linux/amd64` and `linux/arm64`, so they run on every
platform's Docker. The image is distroless, runs as a non-root user, and
carries `LICENSE` and `THIRD_PARTY_NOTICES.md` in `/usr/share/doc/ollametry/`.
Verify it with
`gh attestation verify oci://ghcr.io/a1exus/ollametry:0.1.0 -R a1exus/ollametry`.
How the container reaches Ollama on each OS is in
[container environments](containers.md); to run the image from this
repository's Compose file instead of building, see
[releases](releases.md#using-a-released-image).

## From source

```sh
go install github.com/a1exus/ollametry/cmd/ollametry@latest
ollametry version
```

`go install` needs the Go version in [`go.mod`](../go.mod) and works on every
platform Go supports. The binary reports the module version it was installed
at.
