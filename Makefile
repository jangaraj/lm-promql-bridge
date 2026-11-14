.PHONY: all build run test fmt lint clean tidy deps

BINARY_NAME = lm-promql-bridge
BUILD_DIR = bin

all: clean tidy fmt test build

deps:
	@echo "Ensuring dependencies are up to date..."
	go mod tidy

build: deps
	@echo "Building binary..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) cmd/bridge/main.go

run: deps
	go run cmd/bridge/main.go

test: deps
	@echo "Running tests..."
	go test -v -race ./...

fmt:
	@echo "Formatting code..."
	go fmt ./...
	@if command -v goimports >/dev/null; then \
		goimports -w .; \
	fi

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

tidy:
	@echo "Tidying dependencies..."
	go mod tidy