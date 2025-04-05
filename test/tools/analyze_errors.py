#!/usr/bin/env python3

import json
import re
from pathlib import Path
from typing import Dict, List, Tuple


def extract_error_message(pattern: List[str]) -> str:
    """Extract the actual error message from a pattern."""
    # Join all lines
    full_text = "\\n".join(pattern)

    # Try to find specific error messages
    error_patterns = [
        r"Error: (.*?)(?=\n|$)",  # Standard error format
        r"error: (.*?)(?=\n|$)",  # Lowercase error format
        r"Error validating generated overrides: (.*?)(?=\n|$)",  # Validation error format
        r"helm template error: (.*?)(?=\n|$)",  # Helm error format
        r"Could not locate (.*?)(?=\n|$)",  # File not found error
        r"Error detecting images: (.*?)(?=\n|$)",  # Image detection error
    ]

    for pattern in error_patterns:
        match = re.search(pattern, full_text, re.MULTILINE | re.IGNORECASE)
        if match:
            return match.group(1).strip()

    # If no specific error found, return the first non-debug, non-empty line
    lines = full_text.split("\\n")
    for line in lines:
        line = line.strip()
        if (
            line
            and not line.startswith("[DEBUG]")
            and not line.startswith("Command:")
            and not line.startswith("Stdout:")
            and not line.startswith("Stderr:")
        ):
            return line

    return "Unknown error"


def parse_error_patterns(file_path: str) -> List[Tuple[int, str]]:
    """Parse error patterns file and return list of (count, message) tuples."""
    errors = []
    current_count = 0
    current_pattern = []

    with open(file_path, "r") as f:
        lines = f.readlines()

    in_pattern = False
    for line in lines:
        line = line.strip()
        if (
            not line
            or line.startswith("========")
            or line == "Error Message Patterns and Counts:"
        ):
            continue

        if line.startswith("Count: "):
            if current_count and current_pattern:
                # Extract the actual error message
                error_msg = extract_error_message(current_pattern)
                if error_msg:
                    errors.append((current_count, error_msg))
            current_count = int(line.split(": ")[1])
            current_pattern = []
            in_pattern = False
        elif line == "Pattern:":
            in_pattern = True
        elif in_pattern:
            current_pattern.append(line)

    # Add the last pattern
    if current_count and current_pattern:
        error_msg = extract_error_message(current_pattern)
        if error_msg:
            errors.append((current_count, error_msg))

    return errors


def categorize_errors(errors: List[Tuple[int, str]]) -> Dict[str, Dict]:
    """Categorize errors into meaningful buckets."""
    categories = {
        "image_errors": {
            "patterns": [
                r"repository is not a string",
                r"error detecting images",
                r"error processing key image",
                r"invalid image reference",
                r"registry is not a string",
                r"tag is not a string",
            ],
            "errors": [],
        },
        "missing_required": {
            "patterns": [
                r"must be set",
                r"is required",
                r"cannot be empty",
                r"At least one of .* is required",
                r"cannot be null",
            ],
            "errors": [],
        },
        "validation": {
            "patterns": [
                r"values don't meet the specifications",
                r"validation failed",
                r"invalid YAML syntax",
                r"templates/validation",
                r"validating generated overrides",
                r"execution error at",
            ],
            "errors": [],
        },
        "file_error": {
            "patterns": [
                r"Could not locate",
                r"no such file or directory",
                r"file not found",
            ],
            "errors": [],
        },
        "type_mismatch": {
            "patterns": [
                r"wrong type for value",
                r"Not a table",
                r"cannot overwrite table with non table",
                r"destination for .* is a table",
                r"nil pointer evaluating interface",
                r"skipped value for .*: Not a table",
                r"Invalid type\.",
                r"Expected: \w+, given: \w+",
            ],
            "errors": [],
        },
        "schema_error": {
            "patterns": [
                r"schema\(s\)",
                r"specifications of the schema",
            ],
            "errors": [],
        },
        "library_chart": {
            "patterns": [
                r"library charts are not installable",
            ],
            "errors": [],
        },
        "coalesce_error": {
            "patterns": [
                r"coalesce\.go",
            ],
            "errors": [],
        },
    }

    uncategorized = []

    for count, error in errors:
        categorized = False
        for category, data in categories.items():
            if any(
                re.search(pattern, error, re.IGNORECASE) for pattern in data["patterns"]
            ):
                data["errors"].append((count, error))
                categorized = True
                break
        if not categorized:
            uncategorized.append((count, error))

    # Add uncategorized errors
    categories["uncategorized"] = {"patterns": [], "errors": uncategorized}

    return categories


def analyze_error_file(file_path: str) -> Dict:
    """Analyze error file and return statistics."""
    # Parse errors from the patterns file
    errors = parse_error_patterns(file_path)

    # Categorize errors
    categorized = categorize_errors(errors)

    # Calculate statistics
    total_errors = sum(count for count, _ in errors)
    stats = {
        "total_errors": total_errors,
        "unique_errors": len(errors),
        "categories": {},
    }

    for category, data in categorized.items():
        category_errors = data["errors"]
        if category_errors:
            total_in_category = sum(count for count, _ in category_errors)
            stats["categories"][category] = {
                "total_occurrences": total_in_category,
                "unique_errors": len(category_errors),
                "percentage": (total_in_category / total_errors * 100)
                if total_errors > 0
                else 0,
                "errors": [
                    {"count": count, "message": msg}
                    for count, msg in sorted(
                        category_errors, key=lambda x: x[0], reverse=True
                    )
                ],
            }

    return stats


def generate_report(stats: Dict, output_file: str):
    """Generate a detailed markdown report."""
    with open(output_file, "w") as f:
        f.write("# Error Analysis Report\n\n")

        # Overall statistics
        f.write("## Overall Statistics\n")
        f.write(f"- Total error occurrences: {stats['total_errors']}\n")
        f.write(f"- Unique error patterns: {stats['unique_errors']}\n\n")

        # Categories
        f.write("## Error Categories\n\n")
        sorted_categories = sorted(
            stats["categories"].items(),
            key=lambda x: x[1]["total_occurrences"],
            reverse=True,
        )

        for category, data in sorted_categories:
            f.write(f"### {category.replace('_', ' ').title()}\n")
            f.write(f"- Total occurrences: {data['total_occurrences']}\n")
            f.write(f"- Unique errors: {data['unique_errors']}\n")
            f.write(f"- Percentage of all errors: {data['percentage']:.1f}%\n\n")

            if data["errors"]:
                f.write("#### Top Errors:\n")
                for error in data["errors"]:
                    # Format multiline error messages
                    msg = error["message"].replace("\\n", "\n  ")
                    f.write(f"- ({error['count']}) {msg}\n")
                f.write("\n")


def main():
    # Setup paths
    base_dir = Path(__file__).parent.parent.parent
    test_output_dir = base_dir / "test" / "output"
    error_patterns_file = test_output_dir / "error_patterns.txt"
    report_file = test_output_dir / "error_analysis.md"
    stats_file = test_output_dir / "error_analysis.json"

    # Analyze errors
    stats = analyze_error_file(str(error_patterns_file))

    # Generate reports
    generate_report(stats, str(report_file))

    # Save raw stats
    with open(stats_file, "w") as f:
        json.dump(stats, f, indent=2)

    print("Analysis complete!")
    print(f"- Markdown report: {report_file}")
    print(f"- JSON stats: {stats_file}")


if __name__ == "__main__":
    main()
