.PHONY: build test lint clean run helm-lint test-charts

BINARY_NAME=helm-image-override
BUILD_DIR=bin
GO_FILES=$(shell find . -name "*.go" -type f)
TEST_CHARTS_DIR=test-data/charts
TEST_RESULTS_DIR=test/results
TEST_OVERRIDES_DIR=test/overrides
TARGET_REGISTRY?=harbor.home.arpa

all: lint helm-lint test build

build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/helm-image-override

test:
	@echo "Running unit tests..."
	@go test -v ./...

test-charts: build
	@echo "Running chart tests..."
	@mkdir -p $(TEST_RESULTS_DIR)
	@mkdir -p $(TEST_OVERRIDES_DIR)
	@./test/tools/test-charts.sh $(TARGET_REGISTRY)

lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Skipping lint."; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

helm-lint:
	@echo "Running Helm lint and template validation..."
	@if command -v helm > /dev/null; then \
		echo "Linting test charts..."; \
		helm lint $(TEST_CHARTS_DIR)/minimal-test; \
		echo "Validating templates..."; \
		helm template $(TEST_CHARTS_DIR)/minimal-test > /dev/null; \
		echo "Helm validation complete."; \
	else \
		echo "Helm not installed. Skipping Helm lint and template validation."; \
		echo "Install with: brew install helm (macOS) or follow https://helm.sh/docs/intro/install/"; \
	fi

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf test/charts/
	@rm -rf $(TEST_OVERRIDES_DIR)
	@rm -rf $(TEST_RESULTS_DIR)

run: build
	@echo "Running..."
	@$(BUILD_DIR)/$(BINARY_NAME) $(ARGS) 