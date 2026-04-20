# shellboto — common build/test/release targets
#
# Usage:
#   make build              → bin/shellboto with version info stamped in
#   make build-stripped     → bin/shellboto-stripped (smaller, no debug symbols)
#   make test               → go test ./...
#   make fmt                → gofmt + goimports across the tree
#   make lint               → golangci-lint run
#   make vet                → go vet ./...
#   make vuln               → govulncheck against dependency CVEs
#   make version            → build + run -version to show the stamp
#   make hooks-install      → wire lefthook into .git/hooks
#   make hooks-uninstall    → remove lefthook Git hooks
#   make changelog          → regenerate CHANGELOG.md via git-chglog
#   make release-snapshot   → local goreleaser dry-run (no publish)
#   make release-check      → lint + test + vet + vuln + goreleaser check
#   make tarball            → project tarball at ../shellboto.tar.gz
#   make clean              → rm bin/ dist/

# Version metadata. Falls back to "dev" / "unknown" when not a git
# checkout or git isn't available — makes plain `go build` (without
# this Makefile) trivially distinguishable from a proper build.
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X main.version=$(VERSION) \
           -X main.gitSHA=$(GIT_SHA) \
           -X main.built=$(BUILT)

BIN     := bin/shellboto
STRIPPED := bin/shellboto-stripped

.PHONY: all build build-stripped test fmt lint vet vuln version tarball clean help help-cli install uninstall rollback test-deploy hooks-install hooks-uninstall changelog release-snapshot release-check

all: build

build: ## Build bin/shellboto with version stamp
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/shellboto
	@echo "built $(BIN)  (version=$(VERSION) sha=$(GIT_SHA))"

build-stripped: ## Smaller binary with symbols stripped
	@mkdir -p bin
	go build -ldflags "-s -w $(LDFLAGS)" -o $(STRIPPED) ./cmd/shellboto
	@ls -lh $(STRIPPED)

test: ## Run full test suite
	go test -count=1 -timeout 60s ./...

fmt: ## Format Go sources (gofmt + goimports)
	@command -v goimports >/dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	gofmt -l -w .
	goimports -l -w -local github.com/amiwrpremium/shellboto .

lint: ## Run golangci-lint across the tree
	@command -v golangci-lint >/dev/null 2>&1 || go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	golangci-lint run --timeout=3m

vet: ## Static checks
	go vet ./...

vuln: ## Scan dependencies for known CVEs
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

version: build ## Build + print the embedded version info
	./$(BIN) -version

help-cli: build ## Build + print the CLI subcommand help
	./$(BIN) help

install: ## Run the interactive installer (needs root)
	@./deploy/install.sh

uninstall: ## Run the interactive uninstaller (needs root)
	@./deploy/uninstall.sh

rollback: ## Swap the installed binary with its .prev backup (needs root)
	@./deploy/rollback.sh

test-deploy: ## Unit-test the deploy/lib.sh helpers
	@./deploy/lib_test.sh

hooks-install: ## Wire lefthook into .git/hooks
	@command -v lefthook >/dev/null 2>&1 || { echo "lefthook not found — run ./scripts/install-dev-tools.sh first" >&2; exit 1; }
	lefthook install

hooks-uninstall: ## Remove lefthook Git hooks
	@command -v lefthook >/dev/null 2>&1 && lefthook uninstall || true

changelog: ## Regenerate CHANGELOG.md from commit history
	@command -v git-chglog >/dev/null 2>&1 || go install github.com/git-chglog/git-chglog/cmd/git-chglog@latest
	git-chglog -o CHANGELOG.md

release-snapshot: ## Local goreleaser dry-run; artifacts in dist/
	@command -v goreleaser >/dev/null 2>&1 || go install github.com/goreleaser/goreleaser/v2@latest
	goreleaser release --snapshot --clean

release-check: ## Pre-release sanity gate (lint + test + vet + vuln + goreleaser check)
	$(MAKE) lint
	$(MAKE) test
	$(MAKE) vet
	$(MAKE) vuln
	@command -v goreleaser >/dev/null 2>&1 || go install github.com/goreleaser/goreleaser/v2@latest
	goreleaser check

tarball: ## Tar+gzip the project (sibling to the project dir)
	tar --exclude='*.db' --exclude='*.db-*' --exclude='*.sqlite*' \
	    --exclude='shellboto.lock' --exclude='.DS_Store' \
	    --exclude='bin' --exclude='dist' \
	    -czf ../shellboto.tar.gz -C .. shellboto
	@ls -lh ../shellboto.tar.gz

clean: ## Remove built binaries and packaging output
	rm -rf bin dist

help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
