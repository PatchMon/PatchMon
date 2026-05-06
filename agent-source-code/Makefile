# Makefile for PatchMon Agent

# Build variables
BINARY_NAME=patchmon-agent
BUILD_DIR=build
# Where backend serves agents from (both must stay in sync)
AGENTS_REPO=../agents
AGENTS_DOCKER=../docker/compose_dev_data/agents
# Strip debug info and embed signing public key (required — must be set via PATCHMON_SIGNING_PUBLIC_KEY env var)
SIGNING_KEY_LDFLAG=$(if $(PATCHMON_SIGNING_PUBLIC_KEY),-X patchmon-agent/internal/pkgversion.SigningPublicKey=$(PATCHMON_SIGNING_PUBLIC_KEY),)
LDFLAGS=-ldflags "-s -w $(SIGNING_KEY_LDFLAG)"
# Disable VCS stamping
BUILD_FLAGS=-buildvcs=false

# Go variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(BUILD_DIR)
# Prefer go from PATH; fallback to /usr/local/go/bin for environments that need it
GO_CMD ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
# Prefer golangci-lint from PATH when available
GOLANGCI_LINT_CMD ?= $(shell command -v golangci-lint 2>/dev/null || echo /root/go/bin/golangci-lint)

# Default target
.PHONY: all
all: build

# Fail fast if PATCHMON_SIGNING_PUBLIC_KEY is not set (required for all release builds)
.PHONY: check-signing-key
check-signing-key:
	@[ -n "$(PATCHMON_SIGNING_PUBLIC_KEY)" ] || (echo "❌ Error: PATCHMON_SIGNING_PUBLIC_KEY is not set. Export it before building."; exit 1)

# Build the application (no signing key required for local dev builds)
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) ./cmd/patchmon-agent

# Build Linux binaries only (amd64, 386, arm64, arm)
.PHONY: build-linux
build-linux: check-signing-key
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-amd64 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-386 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-arm64 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-arm ./cmd/patchmon-agent

# Build FreeBSD binaries only (amd64, 386, arm64, arm)
.PHONY: build-freebsd
build-freebsd: check-signing-key
	@mkdir -p $(BUILD_DIR)
	@GOOS=freebsd GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-freebsd-amd64 ./cmd/patchmon-agent
	@GOOS=freebsd GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-freebsd-386 ./cmd/patchmon-agent
	@GOOS=freebsd GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-freebsd-arm64 ./cmd/patchmon-agent
	@GOOS=freebsd GOARCH=arm GOARM=6 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-freebsd-arm ./cmd/patchmon-agent

# Build Windows binaries only (amd64, arm64 — 32-bit Windows is not supported; Win10 x86 hit EOL 2025-10-14)
.PHONY: build-windows
build-windows: check-signing-key
	@mkdir -p $(BUILD_DIR)
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-windows-amd64.exe ./cmd/patchmon-agent
	@GOOS=windows GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-windows-arm64.exe ./cmd/patchmon-agent

# Sign all built binaries in BUILD_DIR using minisign.
# Requires PATCHMON_SIGNING_PRIVATE_KEY (path to private key file) to be set.
.PHONY: sign-all
sign-all: check-signing-key
	@[ -n "$(PATCHMON_SIGNING_PRIVATE_KEY)" ] || (echo "❌ Error: PATCHMON_SIGNING_PRIVATE_KEY (path to private key file) is not set."; exit 1)
	@command -v minisign >/dev/null 2>&1 || (echo "❌ Error: minisign is not installed. See https://jedisct1.github.io/minisign/"; exit 1)
	@echo "Signing agent binaries..."
	@[ -n "$(MINISIGN_PASSWORD)" ] || (echo "❌ Error: MINISIGN_PASSWORD is not set. Export it before running sign-all."; exit 1)
	@for f in $(GOBIN)/$(BINARY_NAME)-linux-* $(GOBIN)/$(BINARY_NAME)-freebsd-* $(GOBIN)/$(BINARY_NAME)-windows-*.exe; do \
		case "$$f" in *.minisig) continue ;; esac; \
		[ -f "$$f" ] || continue; \
		printf '%s\n' "$(MINISIGN_PASSWORD)" | minisign -S -s $(PATCHMON_SIGNING_PRIVATE_KEY) -m "$$f" -x "$$f.minisig" -W && echo "  ✅ Signed $$f" || { echo "  ❌ Failed to sign $$f (wrong password?)"; exit 1; }; \
	done

# Build all (Linux + FreeBSD + Windows), sign, and copy to backend serve dirs
.PHONY: build-all
build-all: build-linux build-freebsd build-windows sign-all
	@mkdir -p $(AGENTS_REPO) $(AGENTS_DOCKER)
	@for f in $(GOBIN)/$(BINARY_NAME)-linux-* $(GOBIN)/$(BINARY_NAME)-freebsd-* $(GOBIN)/$(BINARY_NAME)-windows-*.exe; do \
		[ -f "$$f" ] && cp "$$f" "$$f.minisig" $(AGENTS_REPO)/ && cp "$$f" "$$f.minisig" $(AGENTS_DOCKER)/; \
	done
	@echo "Done. agents/ and docker/compose_dev_data/agents/ updated (binaries + signatures)."

# Build all agent binaries into agents-prebuilt/ (for local Docker backend build)
.PHONY: build-all-for-docker
build-all-for-docker:
	@mkdir -p ../agents-prebuilt
	@$(MAKE) build-all
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-* $(BUILD_DIR)/$(BINARY_NAME)-freebsd-* $(BUILD_DIR)/$(BINARY_NAME)-windows-*.exe ../agents-prebuilt/ 2>/dev/null || true
	@cp $(BUILD_DIR)/$(BINARY_NAME)-linux-*.minisig $(BUILD_DIR)/$(BINARY_NAME)-freebsd-*.minisig $(BUILD_DIR)/$(BINARY_NAME)-windows-*.minisig ../agents-prebuilt/ 2>/dev/null || true
	@echo "Agent binaries and signatures copied to agents-prebuilt/"

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	@$(GO_CMD) mod download
	@$(GO_CMD) mod tidy

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	@$(GO_CMD) test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@$(GO_CMD) test -v -coverprofile=coverage.out ./...
	@$(GO_CMD) tool cover -html=coverage.out -o coverage.html

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@$(GO_CMD) fmt ./...

# Verify code is formatted (fails if not; use in CI / pre-commit)
.PHONY: fmt-check
fmt-check:
	@echo "Checking code formatting..."
	@unformatted=$$(gofmt -l . 2>/dev/null); [ -z "$$unformatted" ] || (echo "Code is not formatted. Run 'make fmt'"; echo "$$unformatted"; exit 1)

# Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	@$(GO_CMD) vet ./...

# Lint code
.PHONY: lint
lint:
	@echo "Linting code..."
	@PATH="/usr/local/bin/go/bin:$$PATH" GOFLAGS="$(BUILD_FLAGS)" $(GOLANGCI_LINT_CMD) run

# Run all checks (format, vet, lint, test) - use before commit
.PHONY: check
check: fmt-check vet lint test

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

# Install the binary to system
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(GOBIN)/$(BINARY_NAME) /usr/local/bin/
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build         Build the application"
	@echo "  build-linux   Build Linux binaries (amd64, 386, arm64, arm)"
	@echo "  build-freebsd Build FreeBSD binaries (amd64, 386, arm64, arm)"
	@echo "  build-windows Build Windows binaries (amd64, arm64)"
	@echo "  build-all     Build Linux + FreeBSD + Windows and copy to agents/"
	@echo "  build-all-for-docker  Build all binaries into agents-prebuilt/ for local Docker build"
	@echo "  deps          Install dependencies"
	@echo "  test          Run tests"
	@echo "  test-coverage Run tests with coverage"
	@echo "  fmt           Format code"
	@echo "  fmt-check     Verify code is formatted"
	@echo "  vet           Run go vet"
	@echo "  lint          Lint code"
	@echo "  check         Run fmt-check, vet, lint, test (pre-commit)"
	@echo "  clean         Clean build artifacts"
	@echo "  install       Install binary to /usr/local/bin"
	@echo "  help          Show this help message"
	@echo ""
