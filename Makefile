.PHONY: build test lint clean run helm-lint test-charts test-integration test-cert-manager test-kube-prometheus-stack test-integration-specific test-integration-debug help dist

BINARY_NAME=irr
BUILD_DIR=bin
HELM_PLUGIN_DIR=build/helm-plugin
GO_FILES=$(shell find . -name "*.go" -type f)
TEST_CHARTS_DIR=test-data/charts
TEST_RESULTS_DIR=test/results
TEST_OVERRIDES_DIR=test/overrides
TARGET_REGISTRY?=harbor.home.arpa
VERSION=$(shell grep -o '^version:[ "]*[^"]*' plugin.yaml | awk '{print $$2}' | tr -d '"')
DIST=$(CURDIR)/_dist
LDFLAGS="-X main.version=$(VERSION)"

# Platform-specific build settings - Keep GOOS/GOARCH available for manual builds if needed
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

all: lint helm-lint test test-integration build

build:
	@echo "Building $(BINARY_NAME) for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags=$(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/irr

# Simplified dist target for packaging - Explicit builds for each platform
dist:
	@echo "Creating distribution packages for all supported platforms..."
	@mkdir -p $(DIST) $(BUILD_DIR)/bin # Ensure dist and bin directories exist

	@echo "Building and packaging for linux/amd64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags=$(LDFLAGS) -o $(BUILD_DIR)/bin/$(BINARY_NAME) ./cmd/irr
	@tar -zcvf $(DIST)/helm-$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz \
		-C $(BUILD_DIR) bin/$(BINARY_NAME) \
		-C $(CURDIR) README.md LICENSE plugin.yaml install-binary.sh

	@echo "Building and packaging for linux/arm64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags=$(LDFLAGS) -o $(BUILD_DIR)/bin/$(BINARY_NAME) ./cmd/irr
	@tar -zcvf $(DIST)/helm-$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz \
		-C $(BUILD_DIR) bin/$(BINARY_NAME) \
		-C $(CURDIR) README.md LICENSE plugin.yaml install-binary.sh

	@echo "Building and packaging for darwin/arm64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags=$(LDFLAGS) -o $(BUILD_DIR)/bin/$(BINARY_NAME) ./cmd/irr
	@tar -zcvf $(DIST)/helm-$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz \
		-C $(BUILD_DIR) bin/$(BINARY_NAME) \
		-C $(CURDIR) README.md LICENSE plugin.yaml install-binary.sh

	@echo "Distribution packages created in $(DIST)"

test: build
	@echo "Running unit tests..."
	@IRR_TESTING=true go test ./... -v 
	@echo "Running CLI syntax tests..."
	@IRR_TESTING=true go test -v ./cmd/irr/cli_test.go

test-json: build
	@echo "Running unit tests..."
	@IRR_TESTING=true go test ./... -json | jq -r 'select((.Action == "fail") and .Test)'

test-packages: build
	@echo "Running package tests (skipping cmd/irr)..."
	@IRR_TESTING=true go test -v ./pkg/... ./test/...

test-cli: build
	@echo "Running CLI syntax tests..."
	@IRR_TESTING=true go test -v ./cmd/irr/cli_test.go

test-pkg-image: build
	@echo "Running image package tests..."
	@IRR_TESTING=true go test -v ./pkg/image/...

test-pkg-override: build
	@echo "Running override package tests..."
	@IRR_TESTING=true go test -v ./pkg/override/...

test-pkg-strategy: build
	@echo "Running strategy package tests..."
	@IRR_TESTING=true go test -v ./pkg/strategy/...

test-integration: build
	@echo "Running integration tests..."
	@IRR_TESTING=true go test -v ./test/integration/...

test-cert-manager: build
	@echo "Running cert-manager component-group tests..."
	@IRR_TESTING=true go test -v ./test/integration/... -run TestCertManager

test-cert-manager-debug: build
	@echo "Running cert-manager component-group tests with debug output..."
	@IRR_TESTING=true LOG_LEVEL=DEBUG IRR_DEBUG=1 go test -v ./test/integration/... -run TestCertManager

test-cert-manager-cores: build
	@echo "Running cert-manager core controllers component test..."
	@IRR_TESTING=true go test -v ./test/integration/... -run TestCertManager/core_controllers

test-kube-prometheus-stack: build
	@echo "Running kube-prometheus-stack component-group tests..."
	@IRR_TESTING=true go test -v ./test/integration/... -run TestKubePrometheusStack

test-kube-prometheus-stack-debug: build
	@echo "Running kube-prometheus-stack component-group tests with debug output..."
	@IRR_TESTING=true LOG_LEVEL=DEBUG IRR_DEBUG=1 go test -v ./test/integration/... -run TestKubePrometheusStack

# You can run a specific integration test with:
# make test-integration-specific TEST_NAME=TestConfigFileMappings
test-integration-specific: build
	@echo "Running specific integration test: $(TEST_NAME)"
	@IRR_TESTING=true go test -v ./test/integration/... -run $(TEST_NAME)

test-integration-debug: build
	@echo "Running integration tests with debug output..."
	@IRR_TESTING=true LOG_LEVEL=DEBUG IRR_DEBUG=1 go test -v ./test/integration/...

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

help:
	@echo "Available targets:"
	@echo "  all                Build and run all tests"
	@echo "  build              Build the irr binary for current host OS/ARCH (or specify GOOS/GOARCH)"
	@echo "  dist               Create distribution tarball for current host OS/ARCH (or specify GOOS/GOARCH)"
	@echo "  helm-lint          Run Helm lint and template validation"
	@echo "  test               Run all unit tests"
	@echo "  test-json          Run unit tests with JSON output"
	@echo "  test-packages      Run package tests (skipping cmd/irr)"
	@echo "  test-cli           Run CLI syntax tests"
	@echo "  test-pkg-image     Run image package tests"
	@echo "  test-pkg-override  Run override package tests"
	@echo "  test-pkg-strategy  Run strategy package tests"
	@echo "  test-integration   Run all integration tests"
	@echo "  test-integration-specific TEST_NAME=TestName  Run a specific integration test"
	@echo "  test-integration-debug  Run integration tests with debug output"
	@echo "  test-cert-manager  Run cert-manager component-group tests"
	@echo "  test-cert-manager-debug  Run cert-manager component-group tests with debug output"
	@echo "  test-cert-manager-cores  Run cert-manager core controllers component test"
	@echo "  test-kube-prometheus-stack  Run kube-prometheus-stack component-group tests"
	@echo "  test-kube-prometheus-stack-debug  Run kube-prometheus-stack tests with debug output"
	@echo "  test-charts        Run chart tests"
	@echo "  lint               Run linter"
	@echo "  helm-lint          Run Helm lint and template validation"
	@echo "  clean              Clean up build artifacts"
	@echo "  run ARGS=\"./..\"     Run the irr binary with the specified arguments"
	@echo "  help               Show this help message"
