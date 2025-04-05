#!/usr/bin/env bash

# Main test script for testing chart template rendering and override generation

# Ensure strict mode
set -euo pipefail

# Source configuration and utilities
# shellcheck source=./lib/config.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/config.sh"
# shellcheck source=./lib/repo-manager.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/repo-manager.sh"
# shellcheck source=./lib/chart-tester.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/chart-tester.sh"
# shellcheck source=./utils/log-utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/utils/log-utils.sh"
# shellcheck source=./utils/json-utils.sh
source "$(dirname "${BASH_SOURCE[0]}")/utils/json-utils.sh"

# Process a single chart and return results
process_chart() {
    local chart="$1"
    local chart_dir="$2"
    local temp_dir
    temp_dir=$(mktemp -d)
    local success_file="${temp_dir}/success"
    
    echo "Processing chart: ${chart}"
    
    # Pull the chart
    if ! pull_chart "${chart}" "${temp_dir}"; then
        log_error "${ANALYSIS_RESULTS_FILE}" \
            "${chart}: Failed to pull chart" \
            "" \
            ""
        rm -rf "${temp_dir}"
        echo "${chart} 0 0"
        return 0
    fi
    
    # Test chart template rendering
    if test_chart "${chart}" "${temp_dir}"; then
        echo "analysis" >> "${success_file}"
    fi
    
    # Test chart override generation
    if test_chart_override "${chart}" "${temp_dir}"; then
        echo "override" >> "${success_file}"
    fi
    
    # Cleanup
    rm -rf "${temp_dir}"
    
    # Return results in format: "chart_name analysis_success override_success"
    if [ -f "${success_file}" ]; then
        analysis_count=$(grep -c "analysis" "${success_file}" || echo "0")
        override_count=$(grep -c "override" "${success_file}" || echo "0")
        echo "${chart} ${analysis_count} ${override_count}"
    else
        echo "${chart} 0 0"
    fi
}

# Check if target registry is provided
if [ $# -lt 1 ]; then
    echo "Usage: $0 <target-registry> [--no-parallel]"
    exit 1
fi

# Set target registry
TARGET_REGISTRY="$1"
export TARGET_REGISTRY

# Check for parallel execution flag
USE_PARALLEL=true
if [ "${2:-}" = "--no-parallel" ]; then
    USE_PARALLEL=false
fi

# Check for GNU Parallel if needed
if [ "${USE_PARALLEL}" = true ]; then
    if ! command -v parallel &> /dev/null; then
        echo "GNU Parallel is required but not installed. Please install it first or use --no-parallel."
        echo "To install on macOS: brew install parallel"
        echo "To install on Ubuntu/Debian: apt-get install parallel"
        exit 1
    fi
fi

# Initialize results files
echo "Creating error patterns file..."
{
    echo "REGISTRY_PREFIX: Double registry prefix in image path"
    echo "IMAGE_VALIDATION: Image validation failed"
    echo "TEMPLATE_ERROR: Chart template rendering failed"
    echo "BITNAMI_SPECIFIC: Bitnami chart specific error"
    echo "PULL_ERROR: Failed to pull chart"
    echo "UNKNOWN: Unknown error"
    echo ""
} > "${ERROR_PATTERNS_FILE}"

# Initialize results files
: > "${ANALYSIS_RESULTS_FILE}"
: > "${OVERRIDE_RESULTS_FILE}"

# Initialize counters
total_charts=0
successful_analysis=0
successful_overrides=0

# Add Helm repositories
echo "Adding Helm repositories..."
add_helm_repositories

# Update repositories sequentially
echo "Updating Helm repositories..."
update_helm_repositories

# Create temporary directory for charts
CHART_DIR=$(mktemp -d)
trap 'rm -rf "${CHART_DIR}"' EXIT

# Export required functions and variables for parallel execution
export -f process_chart pull_chart test_chart test_chart_override log_error categorize_error to_lower get_error_category_description
export CHART_DIR ANALYSIS_RESULTS_FILE OVERRIDE_RESULTS_FILE TARGET_REGISTRY TEST_OUTPUT_DIR DETAILED_LOGS_DIR ERROR_PATTERNS_FILE ERROR_CATEGORY_KEYS ERROR_CATEGORY_DESCRIPTIONS

# Process charts based on execution mode
if [ "${USE_PARALLEL}" = true ]; then
    echo "Processing charts in parallel..."
    # Set number of parallel jobs based on CPU cores, with a minimum of 4 and maximum of 16
    PARALLEL_JOBS=$(( $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4) / 2 ))
    PARALLEL_JOBS=$(( PARALLEL_JOBS < 4 ? 4 : PARALLEL_JOBS > 16 ? 16 : PARALLEL_JOBS ))
    
    # Create a temporary directory for results
    RESULTS_DIR=$(mktemp -d)
    trap 'rm -rf "${RESULTS_DIR}"; rm -rf "${CHART_DIR}"' EXIT
    
    # Create a named pipe for results
    RESULTS_PIPE="${RESULTS_DIR}/results.pipe"
    mkfifo "${RESULTS_PIPE}"
    
    # Start background process to read from pipe and update counters
    {
        while IFS=' ' read -r chart analysis_success override_success; do
            ((total_charts++))
            ((successful_analysis+=analysis_success))
            ((successful_overrides+=override_success))
        done < "${RESULTS_PIPE}"
    } &
    COUNTER_PID=$!
    
    # Process charts in parallel
    list_charts | xargs -P "${PARALLEL_JOBS}" -I{} bash -c \
        'result=$(process_chart "{}" "'"${CHART_DIR}"'"); echo "{} ${result}"' > "${RESULTS_PIPE}"
    
    # Close the pipe and wait for counter process
    exec 3>"${RESULTS_PIPE}"
    exec 3>&-
    wait "${COUNTER_PID}"
    
    # Clean up
    rm -f "${RESULTS_PIPE}"
    rm -rf "${RESULTS_DIR}"
else
    echo "Processing charts sequentially..."
    # Process charts sequentially
    while IFS= read -r chart; do
        ((total_charts++))
        while IFS=' ' read -r _ analysis_success override_success; do
            ((successful_analysis+=analysis_success))
            ((successful_overrides+=override_success))
        done < <(process_chart "${chart}" "${CHART_DIR}")
    done < <(list_charts)
fi

# Generate summary
echo -e "\nTesting complete. Results saved to:"
echo "- Analysis results: ${ANALYSIS_RESULTS_FILE}"
echo "- Override results: ${OVERRIDE_RESULTS_FILE}"
echo "- Summary JSON: ${SUMMARY_JSON_FILE}"
echo "- Error patterns: ${ERROR_PATTERNS_FILE}"
echo "- Detailed logs: ${DETAILED_LOGS_DIR}"
echo -e "\nAnalysis Summary:"
echo "Successful charts: ${successful_analysis}/${total_charts} ($(( total_charts > 0 ? successful_analysis * 100 / total_charts : 0 ))%)"
echo "Failed charts: $(( total_charts - successful_analysis ))/${total_charts}"
echo -e "\nOverride Summary:"
echo "Successful charts: ${successful_overrides}/${total_charts} ($(( total_charts > 0 ? successful_overrides * 100 / total_charts : 0 ))%)"
echo "Failed/Skipped charts: $(( total_charts - successful_overrides ))/${total_charts}"

# Generate error category summary
echo -e "\nError Category Summary:"
for category in "${ERROR_CATEGORY_KEYS[@]}"; do
    # Count occurrences of this category
    count=$(grep -l "Category: ${category}" "${DETAILED_LOGS_DIR}"/*-unknown.log 2>/dev/null | wc -l || echo "0")
    if [ "${count}" -gt 0 ]; then
        echo "- ${category}: ${count} occurrences"
        echo "  Affected charts:"
        for log in $(grep -l "Category: ${category}" "${DETAILED_LOGS_DIR}"/*-unknown.log 2>/dev/null); do
            chart_name=$(basename "${log}" "-unknown.log")
            echo "  * ${chart_name}"
        done
    fi
done

# List failed charts
echo -e "\nFailed Charts:"
for log in "${DETAILED_LOGS_DIR}"/*-unknown.log; do
    if [ -f "${log}" ]; then
        chart_name=$(basename "${log}" "-unknown.log")
        echo "${chart_name}"
    fi
done

# Generate summary JSON
generate_summary_json \
    "${total_charts}" \
    "${successful_analysis}" \
    "${successful_overrides}" \
    "${SUMMARY_JSON_FILE}" 
