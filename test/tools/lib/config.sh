#!/usr/bin/env bash

# Configuration module for test-charts.sh
# Defines all configuration variables and paths

# Ensure strict mode
set -euo pipefail

# Base directory is the workspace root
BASE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)"

# Output directories
TEST_OUTPUT_DIR="${BASE_DIR}/test/output"
DETAILED_LOGS_DIR="${TEST_OUTPUT_DIR}/logs"

# Results files
ANALYSIS_RESULTS_FILE="${TEST_OUTPUT_DIR}/analysis-results.txt"
OVERRIDE_RESULTS_FILE="${TEST_OUTPUT_DIR}/override-results.txt"
SUMMARY_JSON_FILE="${TEST_OUTPUT_DIR}/summary.json"
ERROR_PATTERNS_FILE="${TEST_OUTPUT_DIR}/error-patterns.txt"

# Create output directories if they don't exist
mkdir -p "${TEST_OUTPUT_DIR}"
mkdir -p "${DETAILED_LOGS_DIR}"

# Error categories
ERROR_CATEGORY_KEYS=(
    "REGISTRY_PREFIX"
    "IMAGE_VALIDATION"
    "TEMPLATE_ERROR"
    "BITNAMI_SPECIFIC"
    "PULL_ERROR"
    "UNKNOWN"
)

ERROR_CATEGORY_DESCRIPTIONS=(
    "Double registry prefix in image path"
    "Image validation failed"
    "Chart template rendering failed"
    "Bitnami chart specific error"
    "Failed to pull chart"
    "Unknown error"
)

# Function to get error category description
get_error_category_description() {
    local category="$1"
    local index=0
    for key in "${ERROR_CATEGORY_KEYS[@]}"; do
        if [[ "${key}" == "${category}" ]]; then
            echo "${ERROR_CATEGORY_DESCRIPTIONS[${index}]}"
            return 0
        fi
        ((index++))
    done
    echo "Unknown error"
    return 1
}

# Export variables and functions
export TEST_OUTPUT_DIR
export DETAILED_LOGS_DIR
export ANALYSIS_RESULTS_FILE
export OVERRIDE_RESULTS_FILE
export SUMMARY_JSON_FILE
export ERROR_PATTERNS_FILE
export ERROR_CATEGORY_KEYS
export ERROR_CATEGORY_DESCRIPTIONS
export -f get_error_category_description 