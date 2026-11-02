<!-- markdownlint-disable-next-line MD041 -- shown below the PR title -->
## What and why

<!-- What does this change, and what problem does it solve? Link the issue. -->

## How it was tested

<!-- Tests added, fixtures recorded, manual checks against a real Ollama. -->

## Checklist

- [ ] `make check` passes
- [ ] Tests cover the change at the right layer: unit, integration
      (`-tags integration`) or E2E (recorded fixture for new response shapes)
- [ ] `make e2e` passes if the change affects what reaches Prometheus
- [ ] `CHANGELOG.md` updated under `[Unreleased]`
- [ ] `docs/`, CLAUDE.md and the dashboard updated if metrics, ports,
      variables or make targets changed
- [ ] No prompt or response content in logs, metrics or errors
