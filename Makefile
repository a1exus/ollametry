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
TAIL             ?= 200
CURL             := curl --silent --show-error --fail --max-time 10

GO_VERSION       ?= 1.27.3
GOLANGCI_VERSION ?= v2.15.0
GOVULN_VERSION   ?= v1.9.0
ACTIONLINT_VERSION ?= 1.7.12
ZIZMOR_VERSION   ?= 1.30.1
JQ_VERSION       ?= 1.8.1
GOLICENSES_VERSION ?= v2.0.1
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Toolchains run in containers. Caches are named volumes; CI sets them to
# directories (absolute paths become bind mounts) so actions/cache can keep them.
CACHE_GOMOD      ?= ollametry-gomod
CACHE_GOBUILD    ?= ollametry-gobuild
CACHE_GOLANGCI   ?= ollametry-golangci

DOCKER_RUN := docker run --rm --init \
	-v "$(CURDIR)":/app -w /app \
	-v "$(CACHE_GOMOD)":/go/pkg/mod \
	-v "$(CACHE_GOBUILD)":/root/.cache/go-build \
	-e GOFLAGS=-buildvcs=false
GO       := $(DOCKER_RUN) golang:$(GO_VERSION) go
GOLANGCI := $(DOCKER_RUN) -v "$(CACHE_GOLANGCI)":/root/.cache/golangci-lint \
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

.PHONY: tidy-check
tidy-check: tidy ## Fail if go.mod or go.sum are not tidy
	@git diff --exit-code -- go.mod go.sum || { echo 'tidy-check: run make tidy and commit the result' >&2; exit 1; }

.PHONY: dashboard-check
dashboard-check: ## Validate the Grafana dashboard JSON
	@for f in deploy/grafana/*.json; do \
		docker run --rm -i ghcr.io/jqlang/jq:$(JQ_VERSION) -e '.uid and .title and (.panels | length > 0)' < "$$f" >/dev/null \
			|| { echo "dashboard-check: $$f is not a valid dashboard" >&2; exit 1; }; \
	done

.PHONY: lint-ci
lint-ci: ## Lint GitHub Actions workflows (actionlint, zizmor)
	docker run --rm -v "$(CURDIR)":/repo -w /repo rhysd/actionlint:$(ACTIONLINT_VERSION) -color
	docker run --rm -v "$(CURDIR)":/repo:ro -w /repo ghcr.io/zizmorcore/zizmor:$(ZIZMOR_VERSION) --offline .

.PHONY: notices
notices: ## Regenerate the Go module table in THIRD_PARTY_NOTICES.md (go-licenses)
	$(DOCKER_RUN) -e GOOS=linux golang:$(GO_VERSION) \
		go run github.com/google/go-licenses/v2@$(GOLICENSES_VERSION) report ./cmd/ollametry \
	| awk -F, -f tools/notices.awk > .notices.tmp
	awk '/<!-- BEGIN go-licenses -->/ { print; while ((getline line < ".notices.tmp") > 0) print line; skip = 1; next } \
		/<!-- END go-licenses -->/ { skip = 0 } !skip' THIRD_PARTY_NOTICES.md > .notices.md && mv .notices.md THIRD_PARTY_NOTICES.md
	@rm -f .notices.tmp

.PHONY: notices-check
notices-check: notices ## Fail if THIRD_PARTY_NOTICES.md misses a dependency
	@git diff --exit-code -- THIRD_PARTY_NOTICES.md || { echo 'notices-check: run make notices and commit the result' >&2; exit 1; }

.PHONY: ci-lint
ci-lint: tidy-check lint vuln lint-ci dashboard-check notices-check ## CI lint leg: tidy, lint, vuln, workflows, dashboard, notices

.PHONY: check
check: lint lint-ci dashboard-check test vuln ## Lint code, workflows and dashboard, test and scan (run before every commit)

##@ Lifecycle (docker compose)

.PHONY: up
up: validate ## Build if needed, start detached (up -d) and wait until healthy
	VERSION='$(VERSION)' $(COMPOSE) up --detach --build --wait --wait-timeout $(WAIT_TIMEOUT)

.PHONY: down
down: ## Stop and remove the container and network
	$(COMPOSE) down --remove-orphans

.PHONY: start
start: ## Start the existing, stopped container
	$(COMPOSE) start $(SERVICE)

.PHONY: stop
stop: ## Stop the container without removing it
	$(COMPOSE) stop $(SERVICE)

.PHONY: restart
restart: ## Restart the container (no rebuild; use up after code changes)
	$(COMPOSE) restart $(SERVICE)

.PHONY: build
build: ## Build the image, pulling newer base images
	VERSION='$(VERSION)' $(COMPOSE) build --pull

.PHONY: rebuild
rebuild: ## Build the image from scratch (no layer cache)
	VERSION='$(VERSION)' $(COMPOSE) build --pull --no-cache

.PHONY: clean
clean: ## Remove the container, network and locally built images
	$(COMPOSE) down --remove-orphans --rmi local

##@ Observe

.PHONY: ps
ps: ## Show container status, stopped ones included
	$(COMPOSE) ps --all

.PHONY: top
top: ## Show the processes running in the container
	$(COMPOSE) top $(SERVICE)

.PHONY: images
images: ## List the images used by the project
	$(COMPOSE) images

.PHONY: logs
logs: ## Follow proxy logs (logs --follow; TAIL=200 lines of history)
	$(COMPOSE) logs --follow --tail=$(TAIL) $(SERVICE)

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
