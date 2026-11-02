# Pointing clients at the proxy

[← README](../README.md)

**Only requests that go through the proxy are measured.** Anything that calls
`:11434` directly is invisible to the request metrics (the `/api/ps` gauges
still see every loaded model).

```sh
# Ollama CLI
OLLAMA_HOST=127.0.0.1:11435 ollama run granite4.2:8b

# OpenAI SDKs
export OPENAI_BASE_URL=http://127.0.0.1:11435/v1

# Anthropic SDKs and tools that speak the Messages API
export ANTHROPIC_BASE_URL=http://127.0.0.1:11435
```

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
done for you:

```sh
launchctl setenv OLLAMA_HOST 127.0.0.1:11433   # then restart the Ollama app
PROXY_PORT=11434 OLLAMA_UPSTREAM=http://host.docker.internal:11433 make up
```

To undo it, `launchctl unsetenv OLLAMA_HOST`, restart the Ollama app and run
`make up` without the variables.
