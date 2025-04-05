#!/usr/bin/env bash

# Logging utilities module for test-charts.sh
# Handles all logging operations

# Ensure strict mode
set -euo pipefail

# Source configuration
# shellcheck source=../lib/config.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib/config.sh"

# Initialize results files
init_results_files() {
    for results in "${ANALYSIS_RESULTS_FILE}" "${OVERRIDE_RESULTS_FILE}"; do
        {
            echo "Chart Testing Results"
            echo "==================="
            echo "Target Registry: ${TARGET_REGISTRY}"
            echo "==================="
            echo ""
        } > "${results}"
    done
}

# Function to categorize errors based on patterns
categorize_error() {
    local error_text="$1"
    if grep -q "harbor.home.arpa/harbor.home.arpa" <<< "${error_text}"; then
        echo "REGISTRY_PREFIX"
    elif grep -q "unrecognized container images" <<< "${error_text}"; then
        echo "IMAGE_VALIDATION"
    elif grep -q "helm template.*failed" <<< "${error_text}"; then
        echo "TEMPLATE_ERROR"
    elif grep -q "allowInsecureImages" <<< "${error_text}"; then
        echo "BITNAMI_SPECIFIC"
    elif grep -q "Pull command failed" <<< "${error_text}"; then
        echo "PULL_ERROR"
    else
        echo "UNKNOWN"
    fi
}

# Function to convert string to lowercase
to_lower() {
    echo "$1" | tr '[:upper:]' '[:lower:]'
}

# Function to log error details
log_error() {
    local file="$1"
    local message="$2"
    local error_file="$3"
    local stdout_file="$4"
    
    echo "${message}" >> "${file}"
    echo "Error details:" >> "${file}"
    
    # Get error content
    local error_content=""
    if [ -s "${error_file}" ]; then
        error_content=$(cat "${error_file}")
    else
        error_content="No error output captured"
    fi
    
    # Categorize error
    local error_category
    error_category=$(categorize_error "${error_content}")
    local error_description
    error_description=$(get_error_category_description "${error_category}")
    
    echo "Error Category: ${error_category}" >> "${file}"
    echo "Category Description: ${error_description}" >> "${file}"
    
    # Log the actual error
    echo "${error_content}" >> "${file}"
    
    if [ -n "${stdout_file}" ] && [ -s "${stdout_file}" ]; then
        echo "Debug output:" >> "${file}"
        cat "${stdout_file}" >> "${file}"
    fi
    
    # Save detailed error info to logs directory
    local chart_name
    chart_name=$(basename "${message%%:*}")
    local error_category_lower
    error_category_lower=$(to_lower "${error_category}")
    local error_log="${DETAILED_LOGS_DIR}/${chart_name}-${error_category_lower}.log"
    {
        echo "=== Error Details ==="
        echo "Chart: ${chart_name}"
        echo "Category: ${error_category}"
        echo "Description: ${error_description}"
        echo "Message: ${message}"
        echo "=== Error Content ==="
        echo "${error_content}"
        if [ -n "${stdout_file}" ] && [ -s "${stdout_file}" ]; then
            echo "=== Debug Output ==="
            cat "${stdout_file}"
        fi
    } > "${error_log}"
    
    echo "---" >> "${file}"
}

# Function to create error patterns file
create_error_patterns_file() {
    echo "Creating error patterns file..."
    local index=0
    for category in "${ERROR_CATEGORY_KEYS[@]}"; do
        echo "${category}: ${ERROR_CATEGORY_DESCRIPTIONS[${index}]}" >> "${ERROR_PATTERNS_FILE}"
        ((index++))
    done
}

# Function to print test summary
print_test_summary() {
    local total="$1"
    local analysis_success="$2"
    local override_success="$3"
    local failed_charts=("${@:4}")
    
    echo ""
    echo "Testing complete. Results saved to:"
    echo "- Analysis results: ${ANALYSIS_RESULTS_FILE}"
    echo "- Override results: ${OVERRIDE_RESULTS_FILE}"
    echo "- Summary JSON: ${SUMMARY_JSON}"
    echo "- Error patterns: ${ERROR_PATTERNS_FILE}"
    echo "- Detailed logs: ${DETAILED_LOGS_DIR}"
    echo ""
    echo "Analysis Summary:"
    echo "Successful charts: ${analysis_success}/${total} ($(( analysis_success * 100 / total ))%)"
    echo "Failed charts: $(( total - analysis_success ))/${total}"
    echo ""
    echo "Override Summary:"
    echo "Successful charts: ${override_success}/${total} ($(( override_success * 100 / total ))%)"
    echo "Failed/Skipped charts: $(( total - override_success ))/${total}"
    echo ""
    
    # Print error category summary
    echo "Error Category Summary:"
    local index=0
    for category in "${ERROR_CATEGORY_KEYS[@]}"; do
        count=$(jq -r ".error_categories.\"${category}\" // 0" "${SUMMARY_JSON}")
        if [ "${count}" -gt 0 ]; then
            echo "- ${ERROR_CATEGORY_DESCRIPTIONS[${index}]}: ${count} occurrences"
            echo "  Affected charts:"
            local category_lower
            category_lower=$(to_lower "${category}")
            for log in "${DETAILED_LOGS_DIR}"/*-"${category_lower}".log; do
                if [ -f "${log}" ]; then
                    chart_name=$(basename "${log}" "-${category_lower}.log")
                    echo "  * ${chart_name}"
                fi
            done
        fi
        ((index++))
    done
    echo ""
    
    if [ ${#failed_charts[@]} -gt 0 ]; then
        echo "Failed Charts:"
        printf '%s\n' "${failed_charts[@]}"
        echo ""
    fi
}

# Export functions
export -f init_results_files categorize_error to_lower log_error create_error_patterns_file print_test_summary 