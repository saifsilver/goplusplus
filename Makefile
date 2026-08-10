.PHONY: build test bench load-test fmt lint clean help

APP_NAME := goplusplus

help: ## Display available commands
	@echo "🚀 goplusplus Framework Makefile Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build all core packages and examples
	@echo "🔨 Building all goplusplus packages and examples..."
	GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go build -v ./...
	@echo "✅ Build completed successfully!"

test: ## Run unit tests across all packages
	@echo "🧪 Running unit test suite..."
	GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go test -v ./...
	@echo "✅ All tests passed!"

bench: ## Run Go micro-benchmarks with memory allocations (ns/op, B/op)
	@echo "⚡ Running router micro-benchmarks..."
	GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go test -bench=. -benchmem .

load-test: ## Launch load test server on http://localhost:8080
	@echo "🚀 Starting high-throughput load test server..."
	@echo "   Use 'ab -n 100000 -c 100 http://localhost:8080/api/v1/bench/100' in another terminal to benchmark!"
	GOCACHE=$$(pwd)/.cache GOTMPDIR=$$(pwd)/.tmp TMPDIR=$$(pwd)/.tmp CGO_ENABLED=0 go run ./examples/load_test/main.go

fmt: ## Format Go source code
	@echo "🎨 Formatting Go codebase..."
	go fmt ./...

lint: ## Run go vet code analysis
	@echo "🔍 Running go vet static analysis..."
	go vet ./...

clean: ## Clean build cache and temporary artifacts
	@echo "🧹 Cleaning cache and temporary directories..."
	rm -rf .cache .tmp
	go clean -cache
