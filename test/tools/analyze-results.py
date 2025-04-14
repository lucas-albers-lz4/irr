#!/usr/bin/env python3

import argparse
import json
import os

from solver import (
    compare_results,
    generate_report,
    get_parameter_distribution,
    group_charts_by_minimal_params,
)


def parse_args():
    parser = argparse.ArgumentParser(description="Analyze solver results")
    parser.add_argument(
        "--results", required=True, help="Path to solver results JSON file"
    )
    parser.add_argument(
        "--output", default="analysis", help="Output directory for analysis reports"
    )
    parser.add_argument(
        "--compare", help="Optional second results file to compare with"
    )
    parser.add_argument(
        "--top", type=int, default=5, help="Number of top parameter groups to show"
    )
    return parser.parse_args()


def load_and_normalize_results(results_file):
    """Load solver results and normalize structure if needed."""
    with open(results_file, "r") as f:
        data = json.load(f)

    # Check if results follow the new structure with a 'charts' key
    if "charts" in data:
        return data["charts"]

    # Otherwise assume it's the old format
    return data


def print_success_summary(results):
    """Print a summary of success rates"""
    total = len(results)
    successful = sum(
        1
        for r in results.values()
        if "minimal_params" in r and r["minimal_params"] is not None
    )
    success_rate = (successful / total) * 100 if total > 0 else 0

    print("\n=== SUCCESS SUMMARY ===")
    print(f"Total charts: {total}")
    print(f"Successfully solved: {successful}")
    print(f"Success rate: {success_rate:.2f}%")


def print_parameter_distribution(results):
    """Print distribution of parameters across successful charts"""
    dist = get_parameter_distribution(results)

    print("\n=== PARAMETER DISTRIBUTION ===")
    for param, count in sorted(
        dist["counts"].items(), key=lambda x: x[1], reverse=True
    ):
        print(f"{param}: used in {count} charts ({count / len(results) * 100:.2f}%)")

        # Show top value distributions for this parameter
        values = dist["values"][param]
        for value, val_count in sorted(
            values.items(), key=lambda x: x[1], reverse=True
        )[:3]:
            display_val = value[:30] + "..." if len(value) > 30 else value
            print(
                f"  - {display_val}: {val_count} charts ({val_count / count * 100:.2f}%)"
            )


def print_chart_groups(results, top_n=5):
    """Print groups of charts with the same minimal parameter sets"""
    groups = group_charts_by_minimal_params(results)

    print("\n=== CHART GROUPINGS ===")
    print(f"Found {len(groups)} distinct parameter sets")

    # Sort groups by size (number of charts)
    sorted_groups = sorted(groups.items(), key=lambda x: len(x[1]), reverse=True)

    for i, (param_set, charts) in enumerate(sorted_groups[:top_n]):
        params = json.loads(param_set)
        chart_count = len(charts)
        print(
            f"\nGroup {i + 1}: {chart_count} charts ({chart_count / len(results) * 100:.2f}%)"
        )
        print("Parameters:")
        for k, v in params.items():
            print(f"  {k}: {v}")
        print("Example charts:")
        for chart in charts[:3]:
            chart_name = os.path.basename(chart)
            print(f"  - {chart_name}")
        if len(charts) > 3:
            print(f"  - ... and {len(charts) - 3} more")


def print_comparison(results1, results2):
    """Print comparison between two result sets"""
    comp = compare_results(results1, results2)

    print("\n=== COMPARISON ===")
    print(
        f"First set: {len(results1)} charts, {comp['success_rate1']:.2f}% success rate"
    )
    print(
        f"Second set: {len(results2)} charts, {comp['success_rate2']:.2f}% success rate"
    )

    print(f"\nOnly in first set: {len(comp['only_in_first'])} charts")
    for chart in comp["only_in_first"][:3]:
        print(f"  - {os.path.basename(chart)}")
    if len(comp["only_in_first"]) > 3:
        print(f"  - ... and {len(comp['only_in_first']) - 3} more")

    print(f"\nOnly in second set: {len(comp['only_in_second'])} charts")
    for chart in comp["only_in_second"][:3]:
        print(f"  - {os.path.basename(chart)}")
    if len(comp["only_in_second"]) > 3:
        print(f"  - ... and {len(comp['only_in_second']) - 3} more")

    print(f"\nParameter differences: {len(comp['param_differences'])} charts")
    for i, (chart, diff) in enumerate(list(comp["param_differences"].items())[:3]):
        print(f"\n  {i + 1}. {os.path.basename(chart)}")
        print(f"     First set: {diff['results1']}")
        print(f"     Second set: {diff['results2']}")
    if len(comp["param_differences"]) > 3:
        print(f"  ... and {len(comp['param_differences']) - 3} more differences")


def main():
    args = parse_args()

    # Ensure output directory exists
    os.makedirs(args.output, exist_ok=True)

    # Load results
    print(f"Loading solver results from {args.results}...")
    results = load_and_normalize_results(args.results)

    # Print basic summaries
    print_success_summary(results)
    print_parameter_distribution(results)
    print_chart_groups(results, top_n=args.top)

    # Compare if requested
    if args.compare:
        print(f"\nComparing with {args.compare}...")
        compare_results = load_and_normalize_results(args.compare)
        print_comparison(results, compare_results)

    # Generate full report
    report_file = os.path.join(args.output, "analysis_report.json")
    print(f"\nGenerating full analysis report to {report_file}...")
    generate_report(results, report_file)

    print(f"\nAnalysis complete. Full report saved to {report_file}")


if __name__ == "__main__":
    main()
