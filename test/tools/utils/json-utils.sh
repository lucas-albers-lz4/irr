#!/usr/bin/env bash

# JSON utilities module for test-charts.sh
# Handles all JSON operations

# Ensure strict mode
set -euo pipefail

# Source configuration
# shellcheck source=../lib/config.sh
source "$(dirname "${BASH_SOURCE[0]}")/../lib/config.sh"

# Generate summary JSON file
generate_summary_json() {
    local total="$1"
    local analysis_success="$2"
    local override_success="$3"
    local output_file="$4"
    
    # Create JSON content
    local json_content
    json_content=$(cat << EOF
{
    "total_charts": ${total},
    "analysis_success": ${analysis_success},
    "override_success": ${override_success},
    "error_categories": {
EOF
    )
    
    # Add error categories
    local index=0
    local last_index=$((${#ERROR_CATEGORY_KEYS[@]} - 1))
    for category in "${ERROR_CATEGORY_KEYS[@]}"; do
        local count
        count=$(grep -l "Category: ${category}" "${DETAILED_LOGS_DIR}"/*-unknown.log 2>/dev/null | wc -l || echo "0")
        json_content+="        \"${category}\": ${count}"
        if [ "${index}" -lt "${last_index}" ]; then
            json_content+=","
        fi
        json_content+=$'\n'
        ((index++))
    done
    
    json_content+="    }"$'\n'
    json_content+="}"
    
    echo "${json_content}" > "${output_file}"
}

# Export functions
export -f generate_summary_json 