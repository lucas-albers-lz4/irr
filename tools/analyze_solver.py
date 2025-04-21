#!/usr/bin/env python3

import json
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path


def analyze_solver_results(results_file):
    # Load solver results
    with open(results_file, "r") as f:
        data = json.load(f)

    # Group charts by their parameter sets
    param_groups = defaultdict(list)
    chart_count = 0
    success_count = 0
    empty_params_count = 0
    failed_charts = []
    all_error_details = []  # For detailed error analysis

    # Function to convert parameter dictionary to a stable string representation
    def param_key(params):
        if not params:
            return "empty"
        return json.dumps(sorted(params.items()), sort_keys=True)

    # Group charts by minimal parameter sets
    for chart_path, chart_data in data["charts"].items():
        chart_count += 1
        if "minimal_params" in chart_data and chart_data["minimal_params"] is not None:
            success_count += 1
            params = chart_data["minimal_params"]
            key = param_key(params)
            param_groups[key].append(chart_path)
            if not params:
                empty_params_count += 1
        else:
            # Track failed charts
            error_category = "UNKNOWN_ERROR"
            error_details = ""
            if "tested_params" in chart_data and chart_data["tested_params"]:
                error_category = chart_data["tested_params"][0].get(
                    "category", "UNKNOWN_ERROR"
                )
                error_details = chart_data["tested_params"][0].get("details", "")
                all_error_details.append(error_details)  # Collect all error details
            failed_charts.append((chart_path, error_category, error_details))

    # Sort groups by size (number of charts in each group)
    sorted_groups = sorted(param_groups.items(), key=lambda x: len(x[1]), reverse=True)

    # Print results
    print(f"Total charts: {chart_count}")
    print(
        f"Successfully solved charts: {success_count} ({success_count / chart_count * 100:.2f}%)"
    )
    print(
        f"Charts requiring no parameters: {empty_params_count} ({empty_params_count / success_count * 100:.2f}% of successful)"
    )
    print(f"Unique parameter sets: {len(param_groups)}")
    print("\nTop 10 parameter sets by frequency:")

    for i, (param_str, charts) in enumerate(sorted_groups[:10]):
        if param_str == "empty":
            print(
                f"{i + 1}. Empty parameter set: {len(charts)} charts ({len(charts) / success_count * 100:.2f}% of successful)"
            )
            print(f"   Example charts: {', '.join(Path(c).name for c in charts[:3])}")
        else:
            # For displaying parameter sets, we need to handle the JSON format
            try:
                # Attempt to parse as JSON, assuming it's a dictionary string
                params_dict = dict(json.loads(param_str))
                param_desc = ", ".join(f"{k}={v}" for k, v in params_dict.items())
            except json.JSONDecodeError:
                # If not valid JSON, treat it as a plain string description
                param_desc = param_str
            print(
                f"{i + 1}. {param_desc}: {len(charts)} charts ({len(charts) / success_count * 100:.2f}% of successful)"
            )
            print(f"   Charts: {', '.join(Path(c).name for c in charts)}")

    print("\nParameter frequency:")
    param_count = defaultdict(int)
    for param_str, charts in param_groups.items():
        if param_str != "empty":
            # Fix: properly parse the JSON string back to a dictionary
            try:
                params_list = json.loads(param_str)
                # Convert from list of pairs to dictionary to extract keys
                for k, v in params_list:
                    param_count[k] += len(charts)
            except Exception as e:
                print(f"Error parsing parameter set: {e}")

    for param, count in sorted(param_count.items(), key=lambda x: x[1], reverse=True)[
        :10
    ]:
        print(
            f"{param}: {count} charts ({count / success_count * 100:.2f}% of successful)"
        )

    # Group failed charts by error category
    error_groups = defaultdict(list)
    for chart_path, category, details in failed_charts:
        error_groups[category].append((chart_path, details))

    print("\nError categories:")
    for category, charts_with_details in sorted(
        error_groups.items(), key=lambda x: len(x[1]), reverse=True
    ):
        print(
            f"{category}: {len(charts_with_details)} charts ({len(charts_with_details) / chart_count * 100:.2f}% of total)"
        )
        print("   Example charts:")
        for i, (chart_path, details) in enumerate(charts_with_details[:3]):
            chart_name = Path(chart_path).name
            details_short = details[:100] + "..." if len(details) > 100 else details
            print(f"     {i + 1}. {chart_name}: {details_short}")
        if len(charts_with_details) > 3:
            print(f"     ... and {len(charts_with_details) - 3} more")

    # Perform detailed error message analysis
    analyze_error_messages(all_error_details)

    # Extract and analyze error patterns
    print("\nDetailed Error Pattern Analysis:")

    # Extract common patterns from error messages
    error_patterns = defaultdict(list)
    pattern_counts = Counter()

    # Common patterns to look for
    plugin_pattern = re.compile(r'failed to load plugin at "([^"]+)"')
    required_value_pattern = re.compile(r'required value "([^"]+)" is missing')
    yaml_pattern = re.compile(r"(yaml|error parsing)")
    kube_version_pattern = re.compile(r"version\s+(\d+\.\d+\.\d+)")

    for chart_path, category, details in failed_charts:
        chart_name = Path(chart_path).name

        # Check for plugin errors
        plugin_match = plugin_pattern.search(details)
        if plugin_match:
            pattern = "Plugin load error"
            error_patterns[pattern].append((chart_name, details))
            pattern_counts[pattern] += 1
            continue

        # Check for required value errors
        required_match = required_value_pattern.search(details)
        if required_match:
            missing_value = required_match.group(1)
            pattern = f"Missing required value: {missing_value}"
            error_patterns[pattern].append((chart_name, details))
            pattern_counts[pattern] += 1
            continue

        # Check for YAML errors
        yaml_match = yaml_pattern.search(details.lower())
        if yaml_match:
            pattern = "YAML parsing error"
            error_patterns[pattern].append((chart_name, details))
            pattern_counts[pattern] += 1
            continue

        # Check for Kubernetes version errors
        kube_match = kube_version_pattern.search(details)
        if kube_match:
            version = kube_match.group(1)
            pattern = f"Kubernetes version incompatibility (requires {version})"
            error_patterns[pattern].append((chart_name, details))
            pattern_counts[pattern] += 1
            continue

        # If no specific pattern matched
        pattern = "Other/Unclassified error"
        error_patterns[pattern].append((chart_name, details))
        pattern_counts[pattern] += 1

    # Print specific error patterns
    for pattern, count in pattern_counts.most_common():
        charts = error_patterns[pattern]
        print(
            f"\n{pattern}: {count} charts ({count / len(failed_charts) * 100:.2f}% of failures)"
        )
        print("   Example charts:")
        for i, (chart_name, details) in enumerate(charts[:3]):
            details_short = details[:100] + "..." if len(details) > 100 else details
            print(f"     {i + 1}. {chart_name}: {details_short}")
        if len(charts) > 3:
            print(f"     ... and {len(charts) - 3} more")

    # Analysis of chart types that fail vs succeed
    print("\nChart Type Analysis:")

    # Extract chart types based on name patterns
    chart_types = defaultdict(lambda: {"success": 0, "fail": 0})
    common_components = [
        "prometheus",
        "grafana",
        "loki",
        "elastic",
        "postgres",
        "mysql",
        "redis",
        "mongodb",
        "kafka",
        "nginx",
        "traefik",
        "istio",
        "linkerd",
        "cert-manager",
        "argo",
        "vault",
        "harbor",
        "jenkins",
        "consul",
    ]

    for chart_path, chart_data in data["charts"].items():
        chart_name = Path(chart_path).name.lower()
        chart_type = "other"

        # Try to determine chart type from name
        for component in common_components:
            if component in chart_name:
                chart_type = component
                break

        # Check if successful
        success = (
            "minimal_params" in chart_data and chart_data["minimal_params"] is not None
        )
        if success:
            chart_types[chart_type]["success"] += 1
        else:
            chart_types[chart_type]["fail"] += 1

    # Print chart type statistics
    for chart_type, stats in sorted(
        chart_types.items(), key=lambda x: x[1]["fail"], reverse=True
    ):
        total = stats["success"] + stats["fail"]
        if total > 0 and stats["fail"] > 0:  # Only include types with failures
            fail_rate = stats["fail"] / total * 100
            print(
                f"{chart_type}: {stats['fail']}/{total} failed ({fail_rate:.2f}% fail rate)"
            )


def analyze_error_messages(error_details):
    """Analyze error messages in detail and categorize them by specific patterns"""
    print("\nDetailed Error Message Analysis:")

    # Define specific error patterns to look for
    error_patterns = {
        "library_chart": re.compile(r"library charts are not installable"),
        "kube_version": re.compile(r'chart requires kubeVersion: ([^"\n]+)'),
        "schema_validation": re.compile(
            r"values don\'t meet the specifications of the schema"
        ),
        "additional_property": re.compile(
            r"Additional property ([^ ]+) is not allowed"
        ),
        "required_field": re.compile(r"execution error at \([^)]+\): ([^\n]+)"),
        "missing_value": re.compile(r"\'([^\']+)\' must be set"),
        "execution_error": re.compile(r"execution error at \(([^)]+)\)"),
    }

    # Count occurrences of each pattern
    pattern_counts = defaultdict(int)
    pattern_details = defaultdict(list)

    for error in error_details:
        if not error:  # Skip empty errors
            continue

        # Check each pattern
        matched = False
        for pattern_name, pattern in error_patterns.items():
            match = pattern.search(error)
            if match:
                # Try to extract more detailed information if available
                detail = match.group(1) if match.groups() else "No detail"
                key = f"{pattern_name}: {detail}"
                pattern_counts[key] += 1
                pattern_details[key].append(error)
                matched = True

        # If no specific pattern matched, count it as generic
        if not matched:
            pattern_counts["other"] += 1
            pattern_details["other"].append(error)

    # Print results sorted by count
    print(f"Found {len(pattern_counts)} distinct error types")

    for error_type, count in sorted(
        pattern_counts.items(), key=lambda x: x[1], reverse=True
    ):
        print(
            f"\n{error_type}: {count} errors ({count / len(error_details) * 100:.2f}% of errors)"
        )

        # Print example for first error of this type (truncated)
        if pattern_details[error_type]:
            example = pattern_details[error_type][0]
            example_short = example[:100] + "..." if len(example) > 100 else example
            print(f"  Example: {example_short}")

            # If there are multiple errors with the same pattern but different details
            if len(pattern_details[error_type]) > 1:
                distinct_errors = set()
                for err in pattern_details[error_type][:10]:  # Check first 10 errors
                    # Extract a signature part of the error to count unique variants
                    if "execution error at" in err:
                        match = re.search(r"execution error at \(([^)]+)\)", err)
                        if match:
                            distinct_errors.add(match.group(1))
                    elif "required value" in err:
                        match = re.search(r'required value "([^"]+)"', err)
                        if match:
                            distinct_errors.add(f"Missing: {match.group(1)}")

                if len(distinct_errors) > 1:
                    print(
                        f"  Variants: {len(distinct_errors)} different error messages with this pattern"
                    )
                    for i, variant in enumerate(list(distinct_errors)[:3]):
                        print(f"    {i + 1}. {variant}")
                    if len(distinct_errors) > 3:
                        print(f"    ... and {len(distinct_errors) - 3} more variants")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(f"Usage: {sys.argv[0]} <results_file>")
        sys.exit(1)

    results_file = sys.argv[1]
    analyze_solver_results(results_file)
