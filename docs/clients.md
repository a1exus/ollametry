# Pointing clients at the proxy

[← README](../README.md)

- [Per client](#per-client)
- [OpenAI streaming usage](#openai-streaming-usage)
- [Measuring every client](#measuring-every-client)

**Only requests that go through the proxy are measured.** Anything that calls
`:11434` directly is invisible to the request metrics (the `/api/ps` gauges
still see every loaded model).

## Per client

macOS and Linux (bash, zsh):

```sh
# Ollama CLI
OLLAMA_HOST=127.0.0.1:11435 ollama run granite4.2:8b

# OpenAI SDKs
export OPENAI_BASE_URL=http://127.0.0.1:11435/v1

# Anthropic SDKs and tools that speak the Messages API
export ANTHROPIC_BASE_URL=http://127.0.0.1:11435
```

Windows (PowerShell; for every new session, set them with
`[Environment]::SetEnvironmentVariable(name, value, "User")` instead):

```powershell
$env:OLLAMA_HOST = "127.0.0.1:11435"; ollama run granite4.2:8b
$env:OPENAI_BASE_URL = "http://127.0.0.1:11435/v1"
$env:ANTHROPIC_BASE_URL = "http://127.0.0.1:11435"
```

## OpenAI streaming usage

OpenAI-compatible streams carry token counts only when the client asks for
them:

```json
{"stream": true, "stream_options": {"include_usage": true}}
```

Without it those requests still get duration and time to first token, but no
token usage (see [Limits](metrics.md#limits)).

## Measuring every client

To measure every client without reconfiguring them, move Ollama to another
port and put the proxy on 11434. This changes your Ollama setup, so it is not
done for you. First move Ollama to `127.0.0.1:11433`, then restart it:

| OS | Move Ollama | Undo |
| --- | --- | --- |
| macOS (Ollama app) | `launchctl setenv OLLAMA_HOST 127.0.0.1:11433`, then quit and reopen the app | `launchctl unsetenv OLLAMA_HOST` |
| Linux (`ollama.service`) | `sudo systemctl edit ollama`, add `Environment="OLLAMA_HOST=0.0.0.0:11433"` under `[Service]`, `sudo systemctl restart ollama`; a container cannot reach the host's loopback, so Ollama listens on all interfaces (firewall the port) and Compose needs the [`host-gateway` override](containers.md#linux-docker-engine) | remove the line, restart |
| Windows (Ollama app) | `[Environment]::SetEnvironmentVariable("OLLAMA_HOST", "127.0.0.1:11433", "User")`, then quit Ollama from the tray and reopen it | set it to `$null`, restart Ollama |

With Docker Engine inside WSL 2 instead of Docker Desktop, Ollama has to listen
on all interfaces here too, and ollametry points at the WSL gateway
([WSL 2](containers.md#windows-wsl-2)).

Then put ollametry on 11434:

| ollametry runs as | Setting |
| --- | --- |
| Docker Compose | `PROXY_PORT=11434 OLLAMA_UPSTREAM=http://host.docker.internal:11433 make up` |

Clients that use Ollama's default address now go through ollametry.
