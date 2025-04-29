#!/usr/bin/env python3
"""
Analyzes the JSON output from 'test-charts.py --operation subchart' 
(default: test/output/subchart_analysis_results.json)
to summarize discrepancies between irr analyzer and helm template image findings.
"""

import json
import os
from collections import Counter, defaultdict
from pathlib import Path
import argparse # Added for file path argument

# Define configuration
BASE_DIR = Path(__file__).parent.parent.parent.absolute()
DEFAULT_INPUT_FILE = BASE_DIR / "test" / "output" / "subchart_analysis_results.json"

def analyze_results(results_data):
    """Performs the analysis on the loaded results data."""
    total_charts = len(results_data)
    if total_charts == 0:
        return "Analysis Summary:\n-----------------\nNo chart results found in the input data."

    status_counts = Counter(item['status'] for item in results_data)
    template_extra_kinds = defaultdict(int)
    template_extra_images = Counter()
    analyzer_extra_images = Counter()
    yaml_parse_errors = 0
    template_exec_errors = 0

    for item in results_data:
        status = item['status']
        if status == 'ERROR_TEMPLATE_PARSE':
            yaml_parse_errors += 1
        elif status == 'ERROR_TEMPLATE_EXEC':
            template_exec_errors += 1

        if status in ['TEMPLATE_EXTRA', 'MIXED']:
            for img_info in item.get('images_only_in_template_with_kinds', []):
                image = img_info.get('image')
                kinds = img_info.get('kinds', [])
                if image:
                    template_extra_images[image] += 1
                    for kind in kinds:
                        template_extra_kinds[kind] += 1

        if status in ['ANALYZER_EXTRA', 'MIXED']:
             for image in item.get('images_only_in_analyzer', []):
                  analyzer_extra_images[image] +=1


    # --- Summary Generation ---
    summary = f"""
Subchart Discrepancy Analysis Summary:
----------------------------------------
Total Charts Analyzed: {total_charts}

Status Breakdown:
"""
    for status, count in status_counts.items():
        percentage = (count / total_charts * 100) if total_charts > 0 else 0
        summary += f"- {status}: {count} ({percentage:.1f}%)\n"

    summary += f"\nNote: {template_exec_errors} charts failed during `helm template` execution."
    summary += f"\nNote: {yaml_parse_errors} charts had YAML parsing errors after successful `helm template`."


    if template_extra_kinds:
        summary += "\n\nAnalysis of Images Missed by Analyzer (Found only in Template):\n"
        summary += "-------------------------------------------------------------\n"
        summary += "Prevalence by Kubernetes Kind (Top 10):\n"
        # Sort kinds by frequency
        sorted_kinds = sorted(template_extra_kinds.items(), key=lambda item: item[1], reverse=True)
        for i, (kind, count) in enumerate(sorted_kinds):
             if i >= 10: # Limit to top 10
                 summary += f"- ... and {len(sorted_kinds) - 10} more kinds\n"
                 break
             summary += f"- {kind}: {count} instances\n"

        summary += "\nMost Frequently Missed Images (Top 10):\n"
        # Sort images by frequency
        sorted_images = sorted(template_extra_images.items(), key=lambda item: item[1], reverse=True)
        for i, (image, count) in enumerate(sorted_images):
             if i >= 10: # Limit to top 10
                 summary += f"- ... and {len(sorted_images) - 10} more images\n"
                 break
             summary += f"- {image}: {count} instances\n"
    else:
         summary += "\n\nNo significant images were found only in templates (missed by analyzer).\n"


    if analyzer_extra_images:
        summary += "\n\nAnalysis of Images Found Only by Analyzer (Not in Template):\n"
        summary += "-----------------------------------------------------------\n"
        summary += "Most Frequent Images (Top 10):\n"
         # Sort images by frequency
        sorted_images_analyzer = sorted(analyzer_extra_images.items(), key=lambda item: item[1], reverse=True)
        for i, (image, count) in enumerate(sorted_images_analyzer):
             if i >= 10: # Limit to top 10
                 summary += f"- ... and {len(sorted_images_analyzer) - 10} more images\n"
                 break
             summary += f"- {image}: {count} instances\n"
    else:
         summary += "\n\nNo significant images were found only by the analyzer.\n"


    summary += "\n\nKey Findings & Recommendations for Phase 9.3:\n"
    summary += "---------------------------------------------\n"

    # Add qualitative findings based on the data
    if status_counts.get('TEMPLATE_EXTRA', 0) > 0 or status_counts.get('MIXED', 0) > 0:
        summary += "- Discrepancies exist where `helm template` finds images missed by the current analyzer.\n"
        if template_extra_kinds:
             dominant_kind = sorted_kinds[0][0] if sorted_kinds else "N/A"
             summary += f"- Images missed by the analyzer most frequently appear in '{dominant_kind}' kinds (and others listed above).\n"
        summary += "- This confirms the need for Phase 9.3 to replicate Helm's value merging logic to achieve accurate analysis, especially for subcharts.\n"
    else:
        summary += "- The analysis did not reveal significant cases where the current analyzer misses images found in rendered templates.\n"
        summary += "- However, the errors during template execution/parsing for some charts highlight the need for robust error handling and potentially better default values during testing.\n"

    if status_counts.get('ANALYZER_EXTRA', 0) > 0 or status_counts.get('MIXED', 0) > 0:
         summary += "- There are also cases where the analyzer finds images *not* present in the limited template rendering check. This could be due to:\n"
         summary += "  - Images in non-workload kinds (not checked by the template parser).\n"
         summary += "  - Conditional logic (`if/else`) in templates disabling sections in the default run.\n"
         summary += "  - Potential inaccuracies in the analyzer itself.\n"
         summary += "- Phase 9.3 should ideally resolve these by analyzing the *merged* values Helm actually uses.\n"

    summary += "- Focus for Phase 9.3 should be on correctly loading chart dependencies and merging values (`values.yaml`, subchart `values.yaml`, user values) before analysis.\n"
    summary += "- Tracking the origin of values during merging is critical for generating correct override paths in `irr override`.\n"

    return summary

def main():
    parser = argparse.ArgumentParser(description="Analyze subchart discrepancy results.")
    parser.add_argument(
        "-f", "--file",
        type=Path,
        default=DEFAULT_INPUT_FILE,
        help=f"Path to the subchart analysis JSON file (default: {DEFAULT_INPUT_FILE})"
    )
    args = parser.parse_args()

    input_file = args.file

    if not input_file.exists():
        print(f"Error: Input file not found: {input_file}")
        exit(1)
    
    if input_file.stat().st_size == 0:
        print(f"Warning: Input file is empty: {input_file}")
        results_data = []
    else:
        try:
            with open(input_file, 'r', encoding='utf-8') as f:
                results_data = json.load(f)
            if not isinstance(results_data, list):
                 print(f"Error: Input file does not contain a valid JSON list: {input_file}")
                 exit(1)
        except json.JSONDecodeError as e:
            print(f"Error decoding JSON from file {input_file}: {e}")
            exit(1)
        except Exception as e:
            print(f"Error reading file {input_file}: {e}")
            exit(1)

    # Perform analysis and print summary
    summary_output = analyze_results(results_data)
    print(summary_output)

if __name__ == "__main__":
    main()

