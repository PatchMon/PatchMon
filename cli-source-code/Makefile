.PHONY: build test vet clean

BINARY := patchmon
GO_CMD ?= go

build:
	@echo "Building $(BINARY)..."
	@$(GO_CMD) build -buildvcs=false -o $(BINARY) .

test:
	@$(GO_CMD) test ./...

vet:
	@$(GO_CMD) vet ./...

clean:
	@rm -f $(BINARY)

check: test vet

.PHONY: check
