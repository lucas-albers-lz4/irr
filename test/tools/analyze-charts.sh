#!/bin/bash

# analyze-charts.sh - Analyze multiple Helm charts for image patterns
# Usage: ./analyze-charts.sh [--output-dir DIR] [--mode basic|detailed] CHART_URLS...

set -euo pipefail

# Default values
OUTPUT_DIR="analysis-results"
MODE="detailed"
TEMP_DIR="/tmp/chart-analysis"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --mode)
            MODE="$2"
            shift 2
            ;;
        *)
            break
            ;;
    esac
done

# Ensure we have charts to analyze
if [ $# -eq 0 ]; then
    echo "Usage: $0 [--output-dir DIR] [--mode basic|detailed] CHART_URLS..."
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Create temp directory for chart downloads
mkdir -p "$TEMP_DIR"
trap 'rm -rf "$TEMP_DIR"' EXIT

# Function to extract chart name from URL
get_chart_name() {
    local url="$1"
    basename "$url" | sed 's/\.tgz$//'
}

# Function to download chart
download_chart() {
    local url="$1"
    local output="$2"
    if [[ "$url" == http* ]]; then
        curl -sSL "$url" -o "$output"
    else
        cp "$url" "$output"
    fi
}

# Process each chart
for chart_url in "$@"; do
    echo "Processing chart: $chart_url"
    
    # Get chart name and prepare paths
    chart_name=$(get_chart_name "$chart_url")
    chart_file="$TEMP_DIR/$chart_name.tgz"
    result_file="$OUTPUT_DIR/$chart_name.txt"
    json_file="$OUTPUT_DIR/$chart_name.json"
    
    # Download chart
    download_chart "$chart_url" "$chart_file"
    
    # Run analysis
    echo "Analyzing $chart_name..."
    
    # Generate text output
    helm-image-override analyze \
        --mode "$MODE" \
        --output-format text \
        --output-file "$result_file" \
        "$chart_file"
    
    # Generate JSON output
    helm-image-override analyze \
        --mode "$MODE" \
        --output-format json \
        --output-file "$json_file" \
        "$chart_file"
    
    echo "Results written to:"
    echo "  Text: $result_file"
    echo "  JSON: $json_file"
    echo
done

# Generate summary
echo "Generating summary..."
{
    echo "Chart Analysis Summary"
    echo "====================="
    echo
    for result in "$OUTPUT_DIR"/*.txt; do
        chart_name=$(basename "$result" .txt)
        echo "## $chart_name"
        grep -A 3 "Pattern Summary:" "$result"
        echo
    done
} > "$OUTPUT_DIR/summary.md"

echo "Analysis complete. Summary written to $OUTPUT_DIR/summary.md" 