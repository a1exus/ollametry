# Security policy

## Supported versions

Only the latest release and `main` receive security fixes.

## Reporting a vulnerability

Please report vulnerabilities privately through
[GitHub's private vulnerability reporting](https://github.com/a1exus/ollametry/security/advisories/new),
not in public issues or pull requests.

Include what you found, how to reproduce it, and the impact you expect. You
can expect an acknowledgement within a week and a fix or mitigation plan once
the report is confirmed. Credit is given in the advisory unless you ask
otherwise.

## Scope

ollametry is a reverse proxy that sees every prompt and response sent through
it. Of particular interest:

- prompt or response content leaking into logs, metrics or error messages;
- request smuggling, header injection or anything that changes what reaches Ollama;
- ways to make the proxy exhaust memory or file descriptors;
- the container image running with more privilege than documented.

ollametry has no authentication of its own and is meant to listen on
loopback; exposing it on a network without a protected front end is a
deployment issue, not a vulnerability.
