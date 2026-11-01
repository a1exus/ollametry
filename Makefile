SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS += --warn-undefined-variables --no-builtin-rules

COMPOSE          ?= docker compose
SERVICE          ?= ollametry
PROXY_URL        ?= http://127.0.0.1:11435
ADMIN_URL        ?= http://127.0.0.1:11436
OLLAMA_URL       ?= http://127.0.0.1:11434
WAIT_TIMEOUT     ?= 120
CURL             := curl --silent --show-error --fail --max-time 10

GO_VERSION       ?= 1.27.3
GOLANGCI_VERSION ?= v2.15.0
GOVULN_VERSION   ?= v1.9.0
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Toolchains run in containers; module and build caches live in named volumes.
DOCKER_RUN := docker run --rm --init \
	-v "$(CURDIR)":/app -w /app \
	-v ollametry-gomod:/go/pkg/mod \
	-v ollametry-gobuild:/root/.cache/go-build \
	-e GOFLAGS=-buildvcs=false
GO       := $(DOCKER_RUN) golang:$(GO_VERSION) go
GOLANGCI := $(DOCKER_RUN) -v ollametry-golangci:/root/.cache/golangci-lint \
	golangci/golangci-lint:$(GOLANGCI_VERSION) golangci-lint

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m [VAR=value]\n"} \
		/^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)} \
		/^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Develop

.PHONY: test
test: ## Run unit tests with the race detector
	$(GO) test -race -count=1 ./...

.PHONY: cover
cover: ## Run tests and print per-function coverage
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 25

FUZZTIME ?= 30s
FUZZ_TARGETS := ./internal/dialect:FuzzFramer ./internal/dialect:FuzzDecoders \
	./internal/proxy:FuzzModelFromPrefix

.PHONY: fuzz
fuzz: ## Fuzz each parser for FUZZTIME (default 30s); failures land in testdata/fuzz
	@for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; name=$${t##*:}; \
		echo "==> $$name ($$pkg, $(FUZZTIME))"; \
		$(GO) test -run='^$$' -fuzz="^$$name\$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	done

.PHONY: lint
lint: ## Run golangci-lint (config: .golangci.yml)
	$(GOLANGCI) run ./...

.PHONY: fmt
fmt: ## Format code with gofmt and goimports via golangci-lint
	$(GOLANGCI) fmt ./...

.PHONY: vuln
vuln: ## Scan dependencies and stdlib for known vulnerabilities
	$(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULN_VERSION) ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

.PHONY: check
check: lint test vuln ## Lint, test and scan (run before every commit)

##@ Lifecycle

.PHONY: up
up: validate ## Build the image, start the proxy and wait until healthy
	VERSION='$(VERSION)' $(COMPOSE) up --build --detach --wait --wait-timeout $(WAIT_TIMEOUT)

.PHONY: down
down: ## Stop and remove the container
	$(COMPOSE) down --remove-orphans

.PHONY: restart
restart: ## Restart the proxy
	$(COMPOSE) restart $(SERVICE)

.PHONY: build
build: ## Build the container image
	VERSION='$(VERSION)' $(COMPOSE) build --pull

##@ Observe

.PHONY: ps
ps: ## Show container status
	$(COMPOSE) ps

.PHONY: logs
logs: ## Follow proxy logs
	$(COMPOSE) logs --follow --tail=200 $(SERVICE)

.PHONY: health
health: ## Check Ollama, the proxy and the proxy's readiness
	@printf 'ollama  %s ... ' '$(OLLAMA_URL)'
	@$(CURL) --output /dev/null '$(OLLAMA_URL)/api/version' && echo ok
	@printf 'proxy   %s ... ' '$(PROXY_URL)'
	@$(CURL) --output /dev/null '$(PROXY_URL)/api/version' && echo ok
	@printf 'ready   %s ... ' '$(ADMIN_URL)/readyz'
	@$(CURL) --output /dev/null '$(ADMIN_URL)/readyz' && echo ok

.PHONY: smoke
smoke: ## Send one streamed chat request through the proxy (MODEL=<name>)
	@[[ -n "$${MODEL:-}" ]] || { echo 'smoke: set MODEL, e.g. make smoke MODEL=granite4.2:8b' >&2; exit 1; }
	@curl --silent --show-error --fail --max-time 300 '$(PROXY_URL)/api/chat' \
		--data "{\"model\":\"$${MODEL}\",\"messages\":[{\"role\":\"user\",\"content\":\"Say hi in three words.\"}],\"options\":{\"num_predict\":32}}" \
		| tail -n 1; echo

##@ Config

.PHONY: validate
validate: ## Validate compose config and that the grafana network exists
	$(COMPOSE) config --quiet
	@docker network inspect "$${GRAFANA_NETWORK:-grafana_default}" >/dev/null \
		|| { echo "validate: docker network $${GRAFANA_NETWORK:-grafana_default} not found; start ~/opt/grafana first (make up)" >&2; exit 1; }

.PHONY: config
config: ## Print the merged compose configuration
	$(COMPOSE) config
