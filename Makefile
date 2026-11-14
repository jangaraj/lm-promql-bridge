.PHONY: all build run test fmt format lint clean tidy deps

BINARY_NAME = lm-promql-bridge
BUILD_DIR = bin

# Check and build without modifying source or module files.
all: fmt lint test build

deps:
	@echo "Downloading dependencies..."
	go mod download

build: deps
	@echo "Building binary..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/bridge/main.go

run: deps
	go run cmd/bridge/main.go

test: deps
	@echo "Running tests..."
	go test -v -race ./...

# Verify formatting without changing files.
fmt:
	@echo "Checking Go formatting..."
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "Run 'make format' to fix formatting."; exit 1)
	@if command -v goimports >/dev/null; then \
		files="$$(goimports -l .)"; \
		if [ -n "$$files" ]; then echo "$$files"; echo "Run 'make format' to fix imports."; exit 1; fi; \
	fi

# Apply formatting explicitly when desired.
format:
	@echo "Formatting code..."
	@gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')
	@if command -v goimports >/dev/null; then goimports -w .; fi

lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint is not installed. Skipping..."; \
	fi

clean:
	@echo "Cleaning artifacts..."
	rm -rf $(BUILD_DIR)

# Run explicitly when intentionally updating go.mod/go.sum.
tidy:
	@echo "Tidying dependencies..."
	go mod tidy