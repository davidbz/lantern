.DEFAULT_GOAL := help

# Main package to build/run. With several binaries, use cmd/<name>/ and override: `make build CMD=./cmd/worker BIN=bin/worker`.
CMD ?= ./cmd/lantern
BIN ?= bin/lantern
IMAGE ?= $(notdir $(CURDIR)):dev

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
TEST_FLAGS := -race -shuffle=on -count=1 -timeout=5m
# Minimum total statement coverage; generated mocks and cmd/ (covered by the e2e tests in cmd/) are excluded.
COVERAGE_MIN ?= 90
COVERAGE_EXCLUDE := /internal/mocks/|/cmd/

# Packages in this module; empty until the first .go file exists, so targets below skip instead of failing.
PKGS := $(shell go list ./... 2>/dev/null)
DISALLOWED_LICENSES := forbidden,restricted
OUI_URL := https://standards-oui.ieee.org/oui/oui.csv
OUI_EMBED := internal/oui/data/oui.csv.gz

.PHONY: help build run test test-coverage test-coverage-html mocks mocks-clean mocks-regen mocks-check \
	fmt lint lint-fix vuln licenses tidy tidy-check deps oui docker release-snapshot ci clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Build

build: ## Build CMD (default ./cmd/lantern) into BIN (default bin/lantern)
	@if [ -z "$$(go list $(CMD) 2>/dev/null)" ]; then echo "No main package at $(CMD); skipping build"; exit 0; fi; \
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN) $(CMD) && echo "Built $(BIN)"

run: ## Run CMD
	@go run $(CMD)

docker: ## Build the container image (IMAGE, default <dir>:dev)
	@docker build --build-arg CMD=$(CMD) --build-arg VERSION=$(VERSION) -t $(IMAGE) .

release-snapshot: ## Build a local GoReleaser snapshot into dist/ (requires goreleaser)
	@goreleaser release --snapshot --clean

##@ Test

test: mocks ## Run tests with the race detector and shuffled order
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping tests"
else
	@go test $(TEST_FLAGS) ./...
endif

test-coverage: ## Run tests with coverage into coverage.out; fail below COVERAGE_MIN percent
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping tests"
else
	@go test $(TEST_FLAGS) -covermode=atomic -coverprofile=coverage.out ./...
	@grep -Ev '$(COVERAGE_EXCLUDE)' coverage.out > coverage.out.tmp && mv coverage.out.tmp coverage.out
	@go tool cover -func=coverage.out | awk -v min=$(COVERAGE_MIN) '/^total:/ { \
		total = $$NF + 0; print; \
		if (total < min) { printf "coverage %.1f%% is below %d%%\n", total, min; exit 1 } }'
endif

test-coverage-html: test-coverage ## Render coverage.out to coverage.html
	@[ ! -f coverage.out ] || go tool cover -html=coverage.out -o coverage.html

##@ Mocks

mocks: ## Generate mocks from interfaces registered in .mockery.yaml
	@if grep -qE '^  [^ #]' .mockery.yaml; then go tool mockery; else echo "No packages in .mockery.yaml; skipping mocks"; fi

mocks-clean: ## Remove generated mocks
	@rm -rf internal/mocks

mocks-regen: mocks-clean mocks ## Clean and regenerate all mocks

mocks-check: mocks-regen ## Fail if committed mocks are stale
	@if ! git diff --quiet -- internal/mocks || [ -n "$$(git ls-files --others --exclude-standard -- internal/mocks)" ]; then \
		git status --short -- internal/mocks; echo "Mocks are stale; run 'make mocks-regen' and commit"; exit 1; \
	fi

##@ Quality

fmt: ## Format code with the formatters configured in .golangci.yml
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping fmt"
else
	@golangci-lint fmt
endif

lint: ## Lint code (requires golangci-lint)
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping lint"
else
	@golangci-lint run
endif

lint-fix: ## Lint code and apply auto-fixes
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping lint-fix"
else
	@golangci-lint run --fix
endif

vuln: ## Scan for known vulnerabilities (govulncheck)
ifeq ($(PKGS),)
	@echo "No Go packages yet; skipping govulncheck"
else
	@go tool govulncheck ./...
endif

licenses: ## Fail on dependencies with disallowed licenses
	@go tool go-licenses check ./... --disallowed_types=$(DISALLOWED_LICENSES)

tidy: ## Tidy go.mod/go.sum
	@go mod tidy

tidy-check: ## Fail if go.mod/go.sum are not tidy
	@go mod tidy -diff

deps: ## Download dependencies
	@go mod download

oui: ## Refresh the embedded IEEE vendor registry
	@curl -fsSL -A "lantern (+https://github.com/davidbz/lantern)" $(OUI_URL) | gzip -9n > $(OUI_EMBED).tmp
	@mv $(OUI_EMBED).tmp $(OUI_EMBED) && echo "Updated $(OUI_EMBED)"

ci: lint tidy-check mocks-check build test-coverage vuln licenses ## Run every CI check locally

clean: ## Remove build artifacts
	@rm -rf bin/ dist/ coverage.out coverage.html
