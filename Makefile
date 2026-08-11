.PHONY: build test coverage format-check lint security verify install-tools install-hooks bench load-test fmt clean help

APP_NAME := goplusplus
CACHE_DIR := $(CURDIR)/.cache
TMP_DIR := $(CURDIR)/.tmp
TOOLS_DIR := $(CURDIR)/.tools/bin
COVERAGE_FILE := $(TMP_DIR)/coverage.out
COVERAGE_MIN ?= 55.0
GOVULNCHECK_VERSION := v1.6.0
GOVULNCHECK := $(TOOLS_DIR)/govulncheck
GO_ENV := GOCACHE=$(CACHE_DIR) GOTMPDIR=$(TMP_DIR) TMPDIR=$(TMP_DIR) CGO_ENABLED=0

help: ## Display available commands
	@echo "🚀 goplusplus Framework Makefile Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build all core packages and examples
	@echo "🔨 Building all goplusplus packages and examples..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(GO_ENV) go build -v ./...
	@echo "✅ Build completed successfully!"

test: ## Run unit tests across all packages
	@echo "🧪 Running unit test suite..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(GO_ENV) go test -v ./...
	@echo "✅ All tests passed!"

coverage: ## Run all tests and enforce the repository coverage floor
	@echo "📊 Running test suite with coverage (minimum $(COVERAGE_MIN)%)..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(GO_ENV) go test -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	@coverage="$$(GOCACHE=$(CACHE_DIR) GOTMPDIR=$(TMP_DIR) TMPDIR=$(TMP_DIR) go tool cover -func=$(COVERAGE_FILE) | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}')"; \
		echo "Total coverage: $${coverage}%"; \
		awk -v actual="$${coverage}" -v minimum="$(COVERAGE_MIN)" 'BEGIN { exit !(actual + 0 >= minimum + 0) }' || \
		{ echo "❌ Coverage $${coverage}% is below required $(COVERAGE_MIN)%"; exit 1; }
	@echo "✅ Coverage threshold satisfied!"

format-check: ## Fail when tracked Go files are not gofmt formatted
	@echo "🎨 Checking Go formatting..."
	@unformatted="$$(git ls-files '*.go' | xargs gofmt -l)"; \
		test -z "$${unformatted}" || { echo "❌ Run gofmt on:"; echo "$${unformatted}"; exit 1; }
	@echo "✅ Formatting is clean!"

bench: ## Run Go micro-benchmarks with memory allocations (ns/op, B/op)
	@echo "⚡ Running router micro-benchmarks..."
	GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go test -bench=. -benchmem .

PORT ?= 8089

load-test: ## Launch load test server on http://localhost:$(PORT)
	@echo "🚀 Starting high-throughput load test server on port $(PORT)..."
	@echo "   Use 'ab -n 100000 -c 100 -k http://localhost:$(PORT)/api/v1/bench/100' in another terminal to benchmark!"
	PORT=$(PORT) GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go run ./examples/load_test/main.go

fmt: ## Format Go source code
	@echo "🎨 Formatting Go codebase..."
	go fmt ./...

lint: ## Run go vet code analysis
	@echo "🔍 Running go vet static analysis..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(GO_ENV) go vet ./...

security: ## Verify modules and scan reachable code for known vulnerabilities
	@echo "🔐 Verifying modules and scanning vulnerabilities..."
	go mod verify
	@test -x $(GOVULNCHECK) || { echo "❌ govulncheck is missing; run 'make install-tools'"; exit 1; }
	$(GO_ENV) $(GOVULNCHECK) ./...
	@echo "✅ Security checks passed!"

verify: format-check lint coverage security ## Run the complete local/CI quality gate
	@echo "✅ Quality gate passed!"

install-tools: ## Install pinned development and security tools locally
	@mkdir -p $(TOOLS_DIR)
	GOBIN=$(TOOLS_DIR) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

install-hooks: install-tools ## Enable the tracked Git pre-push hook for this clone
	git config core.hooksPath .githooks
	@echo "✅ Git hooks installed. Every push will run 'make verify'."

clean: ## Clean build cache and temporary artifacts
	@echo "🧹 Cleaning cache and temporary directories..."
	rm -rf .cache .tmp
	go clean -cache
