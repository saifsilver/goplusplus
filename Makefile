.PHONY: build test coverage format-check lint static-analysis docs-check race fuzz-smoke api-compat api-compat-test benchmark-gate security verify install-tools install-hooks bench load-test fmt tag clean help

APP_NAME := goplusplus
CACHE_DIR := $(CURDIR)/.cache
TMP_DIR := $(CURDIR)/.tmp
TOOLS_DIR := $(CURDIR)/.tools/bin
COVERAGE_FILE := $(TMP_DIR)/coverage.out
COVERAGE_CLEAN_FILE := $(TMP_DIR)/coverage.clean.out
COVERAGE_MIN ?= 55.0
GOVULNCHECK_VERSION := v1.6.0
GOVULNCHECK := $(TOOLS_DIR)/govulncheck
STATICCHECK_VERSION := 2025.1.1
STATICCHECK := $(TOOLS_DIR)/staticcheck
APIDIFF_VERSION := v0.0.0-20260811152304-ee035b5b010f
APIDIFF := $(TOOLS_DIR)/apidiff
GO_ENV := GOCACHE=$(CACHE_DIR) GOTMPDIR=$(TMP_DIR) TMPDIR=$(TMP_DIR) CGO_ENABLED=0
RACE_ENV := GOCACHE=$(CACHE_DIR) GOTMPDIR=$(TMP_DIR) TMPDIR=$(TMP_DIR)

# Release tagging: `make tag` bumps the patch version, verifies the version
# update, commits it, and creates an annotated tag. Set VERSION for another release.
VERSION ?=

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
	@rm -f $(COVERAGE_FILE) $(COVERAGE_CLEAN_FILE)
	# Go 1.26 can interleave a shared multi-package profile under parallel package execution.
	$(GO_ENV) go test -count=1 -p=1 -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	@awk 'NF' $(COVERAGE_FILE) > $(COVERAGE_CLEAN_FILE)
	@coverage="$$(GOCACHE=$(CACHE_DIR) GOTMPDIR=$(TMP_DIR) TMPDIR=$(TMP_DIR) go tool cover -func=$(COVERAGE_CLEAN_FILE) | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}')"; \
		rm -f $(COVERAGE_CLEAN_FILE); \
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

static-analysis: ## Run pinned Staticcheck analysis
	@echo "🔍 Running Staticcheck..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	@test -x $(STATICCHECK) || { echo "❌ staticcheck is missing; run 'make install-tools'"; exit 1; }
	STATICCHECK_CACHE=$(CACHE_DIR)/staticcheck $(GO_ENV) $(STATICCHECK) ./...

docs-check: ## Require package and exported API documentation
	@echo "📚 Checking Go documentation coverage..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(GO_ENV) go run ./cmd/doccheck .

race: ## Run the full test suite with the race detector
	@echo "🏁 Running race-detector suite..."
	@mkdir -p $(CACHE_DIR) $(TMP_DIR)
	$(RACE_ENV) go test -race -count=1 ./...

fuzz-smoke: ## Exercise every fuzz target briefly
	@echo "🧬 Running fuzz smoke suite..."
	@sh scripts/fuzz_smoke.sh

api-compat-test: ## Test API-diff normalization rules
	@sh scripts/check_api_compat_test.sh

api-compat: api-compat-test ## Reject unreviewed breaking API changes since v1.11.5
	@echo "🧩 Checking public API compatibility..."
	@sh scripts/check_api_compat.sh

benchmark-gate: ## Enforce router allocation budgets
	@echo "⚡ Checking benchmark allocation budgets..."
	@sh scripts/benchmark_gate.sh

security: ## Verify modules and scan reachable code for known vulnerabilities
	@echo "🔐 Verifying modules and scanning vulnerabilities..."
	go mod verify
	@test -x $(GOVULNCHECK) || { echo "❌ govulncheck is missing; run 'make install-tools'"; exit 1; }
	$(GO_ENV) $(GOVULNCHECK) ./...
	@echo "✅ Security checks passed!"

verify: format-check lint static-analysis docs-check coverage race fuzz-smoke api-compat benchmark-gate security ## Run the complete local/CI quality gate
	@echo "✅ Quality gate passed!"

install-tools: ## Install pinned development and security tools locally
	@mkdir -p $(TOOLS_DIR)
	GOBIN=$(TOOLS_DIR) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	GOBIN=$(TOOLS_DIR) go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	GOBIN=$(TOOLS_DIR) go install golang.org/x/exp/cmd/apidiff@$(APIDIFF_VERSION)

install-hooks: install-tools ## Enable the tracked Git pre-push hook for this clone
	git config core.hooksPath .githooks
	@echo "✅ Git hooks installed. Every push will run 'make verify'."

tag: ## Bump gpp.Version, commit, and tag (next patch by default, or VERSION=vX.Y.Z)
	@set -eu; \
		test -z "$$(git status --porcelain)" || { echo "❌ Commit or stash working-tree changes before tagging."; exit 1; }; \
		latest="$$(git tag --list 'v*' --sort=-v:refname | grep -E '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$' | head -n 1)"; \
		requested="$(VERSION)"; \
		if test -n "$${requested}"; then \
			tag="$${requested#v}"; tag="v$${tag}"; \
		else \
			test -n "$${latest}" || latest="v0.0.0"; \
			base="$${latest#v}"; major="$${base%%.*}"; rest="$${base#*.}"; minor="$${rest%%.*}"; patch="$${rest##*.}"; \
			tag="v$${major}.$${minor}.$$((patch + 1))"; \
		fi; \
		printf '%s\n' "$${tag}" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$' || \
			{ echo "❌ VERSION must be semantic version vMAJOR.MINOR.PATCH."; exit 1; }; \
		! git rev-parse --verify --quiet "refs/tags/$${tag}" >/dev/null || \
			{ echo "❌ Tag $${tag} already exists."; exit 1; }; \
		git var GIT_AUTHOR_IDENT >/dev/null 2>&1 || \
			{ echo "❌ Configure git user.name and user.email before tagging."; exit 1; }; \
		framework_version="$$(sed -nE 's/^const Version = "([^"]+)"/\1/p' gpp.go)"; \
		if test "$${framework_version}" != "$${tag}"; then \
			mkdir -p "$(TMP_DIR)"; \
			backup="$$(mktemp "$(TMP_DIR)/gpp-version-backup.XXXXXX")"; \
			candidate="$$(mktemp "$(TMP_DIR)/gpp-version-candidate.XXXXXX")"; \
			cp gpp.go "$${backup}"; rollback=1; \
			cleanup() { status=$$?; trap - EXIT HUP INT TERM; if test "$${rollback}" = 1; then cp "$${backup}" gpp.go; git reset -q HEAD -- gpp.go; fi; rm -f "$${backup}" "$${candidate}"; exit "$${status}"; }; \
			trap cleanup EXIT; trap 'exit 1' HUP INT TERM; \
			sed "s/^const Version = \"$${framework_version}\"$$/const Version = \"$${tag}\"/" gpp.go > "$${candidate}"; \
			grep -Fqx "const Version = \"$${tag}\"" "$${candidate}" || { echo "❌ Could not update gpp.Version safely."; exit 1; }; \
			mv "$${candidate}" gpp.go; \
			git add gpp.go; \
			git commit -m "chore: release $${tag}"; \
			rollback=0; rm -f "$${backup}"; trap - EXIT HUP INT TERM; \
		fi; \
		git tag -a "$${tag}" -m "GoPlusPlus $${tag}"; \
		echo "✅ Created $${tag}. Publish it with: git push origin $${tag}"

clean: ## Clean build cache and temporary artifacts
	@echo "🧹 Cleaning cache and temporary directories..."
	rm -rf .cache .tmp
	go clean -cache
