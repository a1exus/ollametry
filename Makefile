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
GRAFANA_NETWORK  ?= grafana_default
TAIL             ?= 200
CURL             := curl --silent --show-error --fail --max-time 10

GO_VERSION       ?= 1.27.3
GOLANGCI_VERSION ?= v2.15.0
GOVULN_VERSION   ?= v1.9.0
ACTIONLINT_VERSION ?= 1.7.12
ZIZMOR_VERSION   ?= 1.30.1
JQ_VERSION       ?= 1.8.1
PROMETHEUS_VERSION ?= v3.15.1
GOLICENSES_VERSION ?= v2.0.1
BENCHSTAT_VERSION  ?= v0.0.0-20251208221838-04cf7a2dca90
BENCH_COUNT        ?= 6
OLD                ?= old.txt
NEW                ?= bench.txt
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

# Test layers: unit tests have no build tag; integration tests (modules wired
# over real HTTP with fake Ollama and OTLP endpoints) need -tags integration;
# E2E tests (test/e2e, -tags e2e) need a real Ollama and otel-lgtm.
.PHONY: test
test: ## Run unit and integration tests (hermetic) with the race detector
	$(GO) test -race -count=1 -tags integration ./...

.PHONY: test-unit
test-unit: ## Run unit tests, and every benchmark once so they keep working
	$(GO) test -race -count=1 -bench . -benchtime 1x ./...

.PHONY: bench
bench: ## Run benchmarks (BENCH_COUNT=6 runs each) into bench.txt
	$(GO) test -run '^$$' -bench . -benchmem -count=$(BENCH_COUNT) ./... | tee bench.txt

.PHONY: benchstat
benchstat: ## Compare two benchmark runs: make benchstat OLD=old.txt NEW=bench.txt
	$(GO) run golang.org/x/perf/cmd/benchstat@$(BENCHSTAT_VERSION) $(OLD) $(NEW)

.PHONY: test-integration
test-integration: ## Run integration tests (also compiles in the unit tests)
	$(GO) test -race -count=1 -tags integration ./...

E2E_MODEL           ?= granite4.2:8b
E2E_HERMETIC_MODEL  ?= smollm2:135m
E2E_METRICS_TIMEOUT ?= 90s
E2E_COMPOSE         := $(COMPOSE) --file test/e2e/compose.yaml
# Runs the E2E tests in a container on network $(1) against model $(2).
E2E_RUN = $(DOCKER_RUN) --network '$(1)' -e E2E_MODEL='$(2)' \
	-e E2E_METRICS_TIMEOUT='$(E2E_METRICS_TIMEOUT)' golang:$(GO_VERSION) \
	go test -count=1 -v -tags e2e ./test/e2e

.PHONY: e2e
e2e: validate ## E2E tests against the running stack (make up) and host Ollama (E2E_MODEL=granite4.2:8b)
	$(call E2E_RUN,$(GRAFANA_NETWORK),$(E2E_MODEL))

.PHONY: e2e-hermetic
e2e-hermetic: ## E2E tests on a throwaway Ollama + otel-lgtm + ollametry stack (no host Ollama needed)
	@trap 'status=$$?; [ $$status -eq 0 ] || $(E2E_COMPOSE) logs --no-color ollametry | tail -n 50; $(E2E_COMPOSE) down --volumes --remove-orphans; exit $$status' EXIT; \
	$(E2E_COMPOSE) up --detach --build --wait --wait-timeout 300; \
	$(E2E_COMPOSE) exec -T ollama ollama pull '$(E2E_HERMETIC_MODEL)'; \
	$(call E2E_RUN,ollametry-e2e_default,$(E2E_HERMETIC_MODEL))

.PHONY: cover
cover: ## Run unit and integration tests and print per-function coverage
	$(GO) test -race -count=1 -tags integration -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 25

FUZZTIME ?= 30s
FUZZ_TARGETS ?= ./internal/dialect:FuzzFramer ./internal/dialect:FuzzDecoders \
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

.PHONY: rules-check
rules-check: ## Validate and unit-test the Prometheus rules (promtool)
	docker run --rm -v "$(CURDIR)/deploy/prometheus":/rules:ro -w /rules --entrypoint promtool \
		prom/prometheus:$(PROMETHEUS_VERSION) check rules ollametry-rules.yml
	docker run --rm -v "$(CURDIR)/deploy/prometheus":/rules:ro -w /rules --entrypoint promtool \
		prom/prometheus:$(PROMETHEUS_VERSION) test rules ollametry-rules.test.yml

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

DIST_PLATFORMS ?= darwin/arm64 darwin/amd64 linux/arm64 linux/amd64

.PHONY: dist
dist: ## Build reproducible release archives and SHA256SUMS into dist/ (VERSION=vX.Y.Z)
	$(DOCKER_RUN) -e VERSION='$(VERSION)' -e PLATFORMS='$(DIST_PLATFORMS)' \
		-e SOURCE_DATE_EPOCH="$$(git log -1 --format=%ct)" golang:$(GO_VERSION) sh tools/dist.sh

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

.PHONY: versions-check
versions-check: ## Fail if go.mod, the Makefile and the Dockerfile name different Go versions
	@mod=$$(awk '/^go /{print $$2}' go.mod); \
	docker=$$(sed -n 's/^FROM .*golang:\([0-9.]*\)@.*/\1/p' Dockerfile); \
	if [ "$$mod" != "$(GO_VERSION)" ] || [ "$$docker" != "$(GO_VERSION)" ]; then \
		echo "versions-check: go.mod $$mod, Makefile GO_VERSION $(GO_VERSION), Dockerfile golang $$docker; keep them equal" >&2; exit 1; \
	fi; echo "Go $(GO_VERSION) everywhere"

.PHONY: ci-lint
ci-lint: tidy-check versions-check lint vuln lint-ci dashboard-check rules-check notices-check ## CI lint leg: tidy, versions, lint, vuln, workflows, dashboard, rules, notices

.PHONY: check
check: lint lint-ci dashboard-check rules-check test vuln ## Lint code, workflows and dashboard, test and scan (run before every commit)

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
