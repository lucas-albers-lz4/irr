#!/usr/bin/env bash

# Chart testing module for test-charts.sh
# Handles all chart testing operations

# Ensure strict mode
set -euo pipefail

# Source configuration and utilities
# shellcheck source=./config.sh
source "$(dirname "${BASH_SOURCE[0]}")/config.sh"
# shellcheck source=../utils/log-utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/../utils/log-utils.sh"
# shellcheck source=../utils/json-utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/../utils/json-utils.sh"

# Pull a chart from its repository
pull_chart() {
    local chart="$1"
    local output_dir="$2"
    local chart_name
    chart_name=$(basename "${chart}")
    local stdout_file="${TEST_OUTPUT_DIR}/${chart_name}-pull-stdout.txt"
    local stderr_file="${TEST_OUTPUT_DIR}/${chart_name}-pull-stderr.txt"
    
    echo "Pulling chart: ${chart}"
    
    # Try to pull the chart
    if ! helm pull "${chart}" --untar --untardir "${output_dir}" > "${stdout_file}" 2> "${stderr_file}"; then
        log_error "${ANALYSIS_RESULTS_FILE}" \
            "${chart}: Failed to pull chart" \
            "${stderr_file}" \
            "${stdout_file}"
        return 1
    fi
    
    return 0
}

# Test a chart's template rendering
test_chart() {
    local chart="$1"
    local chart_dir="$2"
    local chart_name
    chart_name=$(basename "${chart}")
    local chart_path="${chart_dir}/${chart_name}"
    local stdout_file="${TEST_OUTPUT_DIR}/${chart_name}-analysis-stdout.txt"
    local stderr_file="${TEST_OUTPUT_DIR}/${chart_name}-analysis-stderr.txt"
    local default_values_file="$(dirname "${BASH_SOURCE[0]}")/default-values.yaml"
    
    echo "Testing chart template: ${chart}"
    
    # Try to render the chart template with default values
    if ! helm template "${chart_path}" -f "${default_values_file}" > "${stdout_file}" 2> "${stderr_file}"; then
        log_error "${ANALYSIS_RESULTS_FILE}" \
            "${chart}: Template rendering failed" \
            "${stderr_file}" \
            "${stdout_file}"
        return 1
    fi
    
    return 0
}

# Test chart override generation
test_chart_override() {
    local chart="$1"
    local chart_dir="$2"
    local chart_name
    chart_name=$(basename "${chart}")
    local chart_path="${chart_dir}/${chart_name}"
    local stdout_file="${TEST_OUTPUT_DIR}/${chart_name}-override-stdout.txt"
    local stderr_file="${TEST_OUTPUT_DIR}/${chart_name}-override-stderr.txt"
    local default_values_file="$(dirname "${BASH_SOURCE[0]}")/default-values.yaml"
    
    echo "Testing chart override: ${chart}"
    
    # Try to generate overrides with default values
    if ! helm-image-override \
        --target-registry "${TARGET_REGISTRY}" \
        --chart "${chart_path}" \
        --values "${default_values_file}" \
        --output-file "${TEST_OUTPUT_DIR}/${chart_name}-values.yaml" \
        > "${stdout_file}" 2> "${stderr_file}"; then
        log_error "${OVERRIDE_RESULTS_FILE}" \
            "${chart}: Override generation failed" \
            "${stderr_file}" \
            "${stdout_file}"
        return 1
    fi
    
    # Check if any images were detected
    if ! grep -q "Detected.*images" "${stdout_file}"; then
        log_error "${OVERRIDE_RESULTS_FILE}" \
            "${chart}: No images detected" \
            "${stderr_file}" \
            "${stdout_file}"
        return 1
    fi
    
    # Check for validation errors
    if grep -q "unrecognized container images" "${stderr_file}"; then
        log_error "${OVERRIDE_RESULTS_FILE}" \
            "${chart}: Image validation failed" \
            "${stderr_file}" \
            "${stdout_file}"
        return 1
    fi
    
    return 0
}

# Export functions
export -f pull_chart test_chart test_chart_override