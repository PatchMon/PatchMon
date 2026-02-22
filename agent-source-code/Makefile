# Makefile for PatchMon Agent

# Build variables
BINARY_NAME=patchmon-agent
BUILD_DIR=build
# Where backend serves agents from (both must stay in sync)
AGENTS_REPO=../agents
AGENTS_DOCKER=../docker/compose_dev_data/agents
# Strip debug info (version comes from internal/version/version.go)
LDFLAGS=-ldflags "-s -w"
# Disable VCS stamping
BUILD_FLAGS=-buildvcs=false

# Go variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/$(BUILD_DIR)
# Use go from PATH
GO_CMD?=go
# Use full path to golangci-lint binary to avoid PATH issues when running as root
GOLANGCI_LINT_CMD=/root/go/bin/golangci-lint

# Default target
.PHONY: all
all: build

# Build the application
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) ./cmd/patchmon-agent

# Build for multiple architectures and write directly to backend serve dirs.
# No separate deploy step - build = overwrite. Always.
.PHONY: build-all
build-all:
	@echo "Building for multiple architectures..."
	@mkdir -p $(BUILD_DIR) $(AGENTS_REPO) $(AGENTS_DOCKER)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-amd64 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-386 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-arm64 ./cmd/patchmon-agent
	@GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-linux-arm ./cmd/patchmon-agent
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-windows-amd64.exe ./cmd/patchmon-agent
	@GOOS=windows GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME)-windows-386.exe ./cmd/patchmon-agent
	@echo "Copying to backend serve locations..."
	@for f in $(GOBIN)/$(BINARY_NAME)-linux-* $(GOBIN)/$(BINARY_NAME)-windows-*.exe; do \
		[ -f "$$f" ] && cp "$$f" $(AGENTS_REPO)/ && cp "$$f" $(AGENTS_DOCKER)/; \
	done
	@echo "Done. agents/ and docker/compose_dev_data/agents/ updated."

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

# Lint code
.PHONY: lint
lint:
	@echo "Linting code..."
	@PATH="/usr/local/bin/go/bin:$$PATH" GOFLAGS="$(BUILD_FLAGS)" $(GOLANGCI_LINT_CMD) run

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
	@echo "  build-all     Build all archs + overwrite agents/ and docker/compose_dev_data/agents/"
	@echo "  deps          Install dependencies"
	@echo "  test          Run tests"
	@echo "  test-coverage Run tests with coverage"
	@echo "  fmt           Format code"
	@echo "  lint          Lint code"
	@echo "  clean         Clean build artifacts"
	@echo "  install       Install binary to /usr/local/bin"
	@echo "  help          Show this help message"
	@echo ""
