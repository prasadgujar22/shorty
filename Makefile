# Shorty — make targets. Run `make help` for the list.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

CLUSTER_NAME ?= shorty
KUBE_CONTEXT ?= k3d-$(CLUSTER_NAME)
REGISTRY     ?= 127.0.0.1:5001
ENV          ?= dev
NAMESPACE    ?= shorty
TAG          ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)$(shell git diff --quiet 2>/dev/null || echo -dirty)
GO_SERVICES  := links-svc redirect-svc analytics-svc
SERVICES     := $(GO_SERVICES) web
PLATFORMS    ?= linux/arm64
BASE_URL     ?= http://localhost:8088
SCENARIO     ?= baseline

export CLUSTER_NAME KUBE_CONTEXT REGISTRY ENV NAMESPACE TAG BASE_URL

##@ Setup
.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[1m<target>\033[0m [VAR=value]\n"} \
	  /^[a-zA-Z_%-]+:.*?##/ { printf "  \033[1m%-18s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

.PHONY: bootstrap
bootstrap: tools cluster-up platform ## Install tools, create the cluster, install the platform (ingress, data, observability)

.PHONY: start
start: ## Resume the stopped cluster (or build everything if none exists), then smoke-test all envs
	@scripts/shortyctl.sh start

.PHONY: stop
stop: ## Pause the whole setup (keeps all data, frees CPU/RAM and ports)
	@scripts/shortyctl.sh stop

.PHONY: tools
tools: ## Install/verify CLI tools (Homebrew on macOS)
	@scripts/tools.sh

.PHONY: cluster-up
cluster-up: ## Create the k3d cluster (1 server + 3 agents) and local registry
	@scripts/cluster-up.sh

.PHONY: cluster-down
cluster-down: ## Delete the k3d cluster and registry
	@scripts/cluster-down.sh

.PHONY: platform
platform: ## Install Traefik, CloudNativePG, monitoring, Loki, Alloy (Helm)
	@scripts/platform.sh

##@ Develop
.PHONY: build
build: ## Build all images for $(PLATFORMS) and push to the local registry
	@for s in $(SERVICES); do scripts/build-image.sh $$s; done

.PHONY: build-%
build-%: ## Build a single service image, e.g. make build-links-svc
	@scripts/build-image.sh $*

.PHONY: test
test: ## Unit tests with coverage for Go services and web
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1
	cd services/web && npm run test --silent

.PHONY: test-integration
test-integration: ## Integration tests against Postgres/Redis/NATS containers
	@scripts/test-integration.sh

.PHONY: lint
lint: ## golangci-lint, svelte-check, eslint, helm lint, kubeconform
	golangci-lint run ./...
	cd services/web && npm run lint --silent && npm run check --silent
	@scripts/lint-manifests.sh

.PHONY: up
up: build deploy smoke ## Build, deploy to the local cluster, smoke test (ENV=dev|ha)

.PHONY: deploy
deploy: ## Helm upgrade --install the umbrella chart with local images (ENV=dev|ha)
	@scripts/deploy.sh

.PHONY: smoke
smoke: ## End-to-end smoke test through the gateway
	@scripts/smoke.sh

.PHONY: status
status: ## Cluster state, Argo CD apps, pod health and URLs
	@scripts/shortyctl.sh status

##@ GitOps
.PHONY: gitops
gitops: ## Install Argo CD and the app-of-apps (dev auto-syncs from main)
	@scripts/gitops.sh

##@ Reliability
.PHONY: load
load: ## k6 load test against the redirect path (RATE, DURATION, HOST)
	@scripts/load.sh

.PHONY: chaos
chaos: ## Run all chaos scenarios under k6 load against NS=shorty-ha, append to docs/RESULTS.md
	@chaos/run-all.sh

.PHONY: chaos-%
chaos-%: ## Run one chaos scenario under load, e.g. make chaos-kill-nats
	@chaos/run.sh $*

##@ Housekeeping
.PHONY: dashboards
dashboards: ## Regenerate Grafana dashboard JSON
	node deploy/dashboards/generate.mjs

.PHONY: clean
clean: ## Remove build output, coverage, and delete the cluster
	rm -rf bin dist coverage.out services/web/build services/web/.svelte-kit
	@scripts/cluster-down.sh
