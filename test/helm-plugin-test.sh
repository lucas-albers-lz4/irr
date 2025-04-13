#!/bin/bash

# Test script for the Helm plugin

set -e

# Get the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Build the IRR binary and Helm plugin
cd "$PROJECT_ROOT"
make helm-plugin

# Set up test environment
echo "Setting up test environment..."
TEST_DIR="${PROJECT_ROOT}/tmp/helm-plugin-test"
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

# Install test chart
echo "Installing test chart..."
cp -r "${PROJECT_ROOT}/test-data/charts/minimal-test" .

# Function to check command success
check_command() {
    local cmd="$1"
    local desc="$2"
    echo -n "Testing $desc... "
    if eval "$cmd" > /dev/null 2>&1; then
        echo "Success"
        return 0
    else
        echo "Failed"
        echo "Command: $cmd"
        eval "$cmd"
        return 1
    fi
}

# Test plugin script directly
echo "Testing plugin script execution..."
"${PROJECT_ROOT}/build/helm-plugin/bin/irr" --help > /dev/null
echo "Success"

# Test helm plugin analyze with chart path
check_command "${PROJECT_ROOT}/build/helm-plugin/bin/irr inspect --chart-path ./minimal-test" "plugin inspect with chart path"

# Test helm plugin override with chart path
check_command "${PROJECT_ROOT}/build/helm-plugin/bin/irr override --chart-path ./minimal-test --target-registry registry.example.com --source-registries docker.io --output-file override.yaml" "plugin override with chart path"

# Verify override.yaml was created
if [ -f override.yaml ]; then
    echo "override.yaml was created successfully"
    cat override.yaml
else
    echo "Error: override.yaml was not created"
    exit 1
fi

# Test helm plugin validate with chart path
check_command "${PROJECT_ROOT}/build/helm-plugin/bin/irr validate --chart-path ./minimal-test --values override.yaml" "plugin validate with chart path"

echo ""
echo "All tests passed!"
echo "The Helm plugin has been built and tested successfully." 