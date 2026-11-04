# EnvGuard Makefile. Paths are kept relative so the repo works from
# directories containing spaces.

BINARY            := envguard
PKG               := ./cmd/envguard
BIN_DIR           := bin
DIST_DIR          := dist
GOLANGCI_VERSION  := v2.15.0
GOLANGCI          := $(BIN_DIR)/golangci-lint
GORELEASER_VERSION := v2.19.0
GORELEASER        := $(BIN_DIR)/goreleaser
FUZZTIME          ?= 30s
# Go minimizes every new interesting input for up to 60s by default, which
# stalls short fuzz runs at 0 execs/sec. Keep it short.
FUZZMINIMIZETIME  ?= 1s

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# Steps a local snapshot cannot or should not run: signing needs Sigstore
# OIDC (CI only), Docker images need a running daemon, SBOMs need syft.
SNAPSHOT_SKIP := sign
SNAPSHOT_SKIP := $(SNAPSHOT_SKIP)$(shell docker info >/dev/null 2>&1 || echo ,docker)
SNAPSHOT_SKIP := $(SNAPSHOT_SKIP)$(shell command -v syft >/dev/null 2>&1 || echo ,sbom)

.PHONY: all build test lint security fuzz cover snapshot release-check npm shellcheck demo clean

all: lint test build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

test:
	go test -race ./...

$(GOLANGCI):
	GOBIN="$$(pwd)/$(BIN_DIR)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)

lint: $(GOLANGCI)
	$(GOLANGCI) run ./...

$(BIN_DIR)/gosec:
	GOBIN="$$(pwd)/$(BIN_DIR)" go install github.com/securego/gosec/v2/cmd/gosec@latest

$(BIN_DIR)/govulncheck:
	GOBIN="$$(pwd)/$(BIN_DIR)" go install golang.org/x/vuln/cmd/govulncheck@latest

# Static security analysis (every gosec finding is either fixed or justified in
# a `#nosec` comment; test helpers are excluded) and known vulnerabilities in
# the code EnvGuard actually calls. Needs the network once, to install the tools.
security: $(BIN_DIR)/gosec $(BIN_DIR)/govulncheck
	$(BIN_DIR)/gosec -quiet -exclude-dir=testdata -exclude-dir=testutil -exclude-dir=bin -exclude-dir=dist ./...
	$(BIN_DIR)/govulncheck ./...

# Runs every Fuzz* target in the module for FUZZTIME each.
fuzz:
	@found=0; \
	for file in $$(grep -rl --include='*_test.go' '^func Fuzz' cmd internal docs 2>/dev/null); do \
		dir=$$(dirname "$$file"); \
		for target in $$(grep -o '^func Fuzz[A-Za-z0-9_]*' "$$file" | sed 's/^func //'); do \
			found=1; \
			echo "==> $$dir $$target ($(FUZZTIME))"; \
			go test -run='^$$' -fuzz="^$$target$$" -fuzztime=$(FUZZTIME) -fuzzminimizetime=$(FUZZMINIMIZETIME) "./$$dir" || exit 1; \
		done; \
	done; \
	if [ $$found -eq 0 ]; then echo "no fuzz targets yet"; fi

cover:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1
	go tool cover -html=coverage.out -o coverage.html

$(GORELEASER):
	GOBIN="$$(pwd)/$(BIN_DIR)" go install github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

# Local dry run of the whole release: archives, checksums, SBOMs, deb/rpm/apk,
# Homebrew cask and Scoop manifest (written to dist/, not pushed), Docker
# images (built locally, not pushed) and the npm packages. Publishes nothing.
snapshot: $(GORELEASER)
	$(GORELEASER) release --snapshot --clean --skip=$(SNAPSHOT_SKIP)
	@if command -v node >/dev/null 2>&1; then \
		node scripts/npm/build.mjs --dist $(DIST_DIR) --out $(DIST_DIR)/npm --smoke; \
	else echo "node not found: npm packages not built"; fi

# Validate .goreleaser.yaml (deprecated keys fail too).
release-check: $(GORELEASER)
	$(GORELEASER) check

# Rebuild the npm packages from an existing dist/ (after `make snapshot`).
npm:
	node scripts/npm/build.mjs --dist $(DIST_DIR) --out $(DIST_DIR)/npm --smoke

shellcheck:
	shellcheck install.sh

# Render docs/demo.gif from docs/demo.tape with charmbracelet/vhs (brew install vhs).
demo: build
	@command -v vhs >/dev/null 2>&1 || { echo "vhs is not installed: https://github.com/charmbracelet/vhs (brew install vhs)"; exit 1; }
	vhs docs/demo.tape

clean:
	rm -f $(BIN_DIR)/$(BINARY) coverage.out coverage.html
	rm -rf $(DIST_DIR)
