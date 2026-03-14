.PHONY: build build-migrate build-linux test test-coverage fmt fmt-check vet lint check clean run run-pprof profile-heap profile-allocs sqlc-generate migrate-up migrate-down migrate-version help

BINARY := patchmon-server
MIGRATE_BINARY := migrate
BUILD_DIR := .
# Output dir for multi-arch builds (e.g. ../backend for non-Docker use)
BUILD_OUT_DIR ?= $(BUILD_DIR)
LDFLAGS := -ldflags "-s -w"
BUILD_FLAGS := -buildvcs=false
GO_CMD ?= $(shell command -v go 2>/dev/null || echo go)
GOLANGCI_LINT_CMD ?= $(shell command -v golangci-lint 2>/dev/null || echo golangci-lint)

# Default target
all: build

sqlc-generate:
	@echo "Generating sqlc code..."
	@$(GO_CMD) run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate

build:
	@echo "Building $(BINARY)..."
	@$(GO_CMD) build -buildvcs=false -o $(BINARY) ./cmd/server

build-migrate:
	@echo "Building $(MIGRATE_BINARY)..."
	@$(GO_CMD) build -buildvcs=false -o $(MIGRATE_BINARY) ./cmd/migrate

# Build Linux binaries for all 4 archs (for non-Docker: systemd, bare metal, distro)
.PHONY: build-linux
build-linux:
	@mkdir -p $(BUILD_OUT_DIR)
	@echo "Building patchmon-server (linux amd64, 386, arm64, arm)..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(BINARY)-linux-amd64 ./cmd/server
	@GOOS=linux GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(BINARY)-linux-386 ./cmd/server
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(BINARY)-linux-arm64 ./cmd/server
	@GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(BINARY)-linux-arm ./cmd/server
	@echo "Building migrate (linux amd64, 386, arm64, arm)..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(MIGRATE_BINARY)-linux-amd64 ./cmd/migrate
	@GOOS=linux GOARCH=386 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(MIGRATE_BINARY)-linux-386 ./cmd/migrate
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(MIGRATE_BINARY)-linux-arm64 ./cmd/migrate
	@GOOS=linux GOARCH=arm GOARM=6 CGO_ENABLED=0 $(GO_CMD) build $(BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_OUT_DIR)/$(MIGRATE_BINARY)-linux-arm ./cmd/migrate

test:
	@echo "Running tests..."
	@$(GO_CMD) test -v ./... -count=1

test-coverage:
	@echo "Running tests with coverage..."
	@$(GO_CMD) test -v -coverprofile=coverage.out ./...
	@$(GO_CMD) tool cover -html=coverage.out -o coverage.html

# Format code (modifies files)
fmt:
	@echo "Formatting code..."
	@$(GO_CMD) fmt ./...

# Verify code is formatted (fails if not; use in CI)
fmt-check:
	@echo "Checking code formatting..."
	@unformatted=$$(gofmt -l . 2>/dev/null); [ -z "$$unformatted" ] || (echo "Code is not formatted. Run 'make fmt'"; echo "$$unformatted"; exit 1)

vet:
	@echo "Running go vet..."
	@$(GO_CMD) vet ./...

lint:
	@echo "Linting code..."
	@$(GOLANGCI_LINT_CMD) run ./...

# Run all checks (format, vet, lint, test) - use before commit
check: fmt-check vet lint test

clean:
	@echo "Cleaning build artifacts..."
	@rm -f $(BINARY) $(MIGRATE_BINARY)
	@rm -f $(BUILD_OUT_DIR)/$(BINARY)-linux-* $(BUILD_OUT_DIR)/$(MIGRATE_BINARY)-linux-* 2>/dev/null || true
	@rm -f coverage.out coverage.html

run: build
	./$(BINARY)

# Run with pprof enabled (dev)
run-pprof: build
	ENABLE_PPROF=true ./$(BINARY)

# Fetch heap profile (server must be running with pprof on port 3001)
profile-heap:
	@curl -s "http://localhost:3001/debug/pprof/heap" -o heap.prof
	@echo "Saved to heap.prof. View with: go tool pprof -http=:8080 heap.prof"

# Fetch allocs profile
profile-allocs:
	@curl -s "http://localhost:3001/debug/pprof/allocs" -o allocs.prof
	@echo "Saved to allocs.prof"

# Database migrations (requires DATABASE_URL)
migrate-up:
	@test -n "$$DATABASE_URL" || (echo "DATABASE_URL is required" && exit 1)
	$(GO_CMD) run ./cmd/migrate up

migrate-down:
	@test -n "$$DATABASE_URL" || (echo "DATABASE_URL is required" && exit 1)
	$(GO_CMD) run ./cmd/migrate down

migrate-version:
	@test -n "$$DATABASE_URL" || (echo "DATABASE_URL is required" && exit 1)
	$(GO_CMD) run ./cmd/migrate version

help:
	@echo "Available targets:"
	@echo "  build          Build the application"
	@echo "  build-migrate  Build the migrate CLI binary"
	@echo "  build-linux    Build server+migrate for linux amd64,386,arm64,arm (non-Docker)"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage"
	@echo "  fmt            Format code"
	@echo "  fmt-check      Verify code is formatted (CI)"
	@echo "  vet            Run go vet"
	@echo "  lint           Lint code (requires golangci-lint)"
	@echo "  check          Run fmt-check, vet, lint, test (pre-commit)"
	@echo "  clean          Clean build artifacts"
	@echo "  run            Build and run the server"
	@echo "  run-pprof      Build and run with pprof enabled"
	@echo "  profile-heap   Fetch heap profile (server must be running with pprof)"
	@echo "  profile-allocs Fetch allocs profile"
	@echo "  sqlc-generate  Generate sqlc code"
	@echo "  migrate-up     Run database migrations up"
	@echo "  migrate-down   Run database migrations down"
	@echo "  migrate-version Show migration version"
	@echo "  help           Show this help message"
