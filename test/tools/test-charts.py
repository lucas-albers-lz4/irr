#!/usr/bin/env python3
"""
Test script for irr (Image Registry Rewrite).

This script tests the override generation and template validation for Helm charts.
It assumes charts have already been downloaded to the cache directory (e.g., by pull-charts.py).
It does NOT pull charts from repositories.
"""

import argparse
import asyncio
import csv
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import List, Optional, Tuple, Dict, Set

import yaml

# Define configuration
BASE_DIR = Path(__file__).parent.parent.parent.absolute()
TEST_OUTPUT_DIR = BASE_DIR / "test" / "output"
DETAILED_LOGS_DIR = TEST_OUTPUT_DIR / "logs"
ANALYSIS_RESULTS_FILE = TEST_OUTPUT_DIR / "analysis-results.txt"
OVERRIDE_RESULTS_FILE = TEST_OUTPUT_DIR / "override-results.txt"
SUMMARY_JSON_FILE = TEST_OUTPUT_DIR / "summary.json"
ERROR_PATTERNS_FILE = TEST_OUTPUT_DIR / "error-patterns.txt"
DEFAULT_VALUES_CONTENT = """
global:
  imageRegistry: "harbor.home.arpa/docker"
  imagePullSecrets: []
  storageClass: ""
  security:
    allowInsecureImages: true  # Required for Bitnami charts

# Common image settings
image:
  registry: "harbor.home.arpa/docker"
  repository: ""  # Will be set by the chart
  tag: ""  # Will be set by the chart
  pullPolicy: IfNotPresent
  security:
    allowInsecureImages: true

# Additional common settings
commonAnnotations: {}
commonLabels: {}

# Bitnami specific configuration
registry:
  enabled: true
  server: "harbor.home.arpa/docker"
  security:
    allowInsecureImages: true
"""

# Error categories
ERROR_CATEGORIES = {
    "PULL_ERROR": "Helm chart pull/fetch failed",
    "SETUP_ERROR": "Chart path or prerequisite error",
    "OVERRIDE_ERROR": "irr command failed",
    "TEMPLATE_ERROR": "Helm template rendering failed",
    "SCHEMA_ERROR": "Chart schema validation error",
    "YAML_ERROR": "YAML parsing or syntax error",
    "REQUIRED_VALUE_ERROR": "Required chart value is missing",
    "COALESCE_ERROR": "Type mismatch in value coalescing",
    "LIBRARY_ERROR": "Library chart installation attempted",
    "DEPRECATED_ERROR": "Chart is deprecated",
    "CONFIG_ERROR": "Chart configuration error",
    "TIMEOUT_ERROR": "Processing timed out",
    "KUBERNETES_VERSION_ERROR": "Kubernetes version compatibility error",
    "UNKNOWN_ERROR": "Unclassified error",
}

# Add new constants at the top after existing ones
CHART_CACHE_DIR = BASE_DIR / "test" / "chart-cache"
HELM_CACHE_DIR = Path.home() / "Library" / "Caches" / "helm" / "repository"

# Placeholder for dynamic replacement
TARGET_REGISTRY_PLACEHOLDER = "__TARGET_REGISTRY__"

# Define base values templates for different classifications

# VALUES_TEMPLATE_BITNAMI: Focused on Bitnami common chart patterns
VALUES_TEMPLATE_BITNAMI = f"""
global:
  imageRegistry: {TARGET_REGISTRY_PLACEHOLDER}
  imagePullSecrets: []
  storageClass: ""
  # Ensure allowInsecureImages is set correctly for Bitnami global scope
  security:
    allowInsecureImages: true

# Bitnami often requires specific auth sections, keep minimal dummies
auth:
  # Common placeholders, might be chart specific (e.g., postgresqlPassword, adminPassword)
  password: "dummyPassword"
  rootPassword: "dummyPassword"
  replicaPassword: "dummyReplPassword"

# Bitnami charts often use a standard service structure
service:
  type: ClusterIP
  port: 80 # Keep a basic port, sub-keys added if needed by chart

# Minimal common labels/annotations for Bitnami
commonLabels: {{}}
commonAnnotations: {{}}

# Explicitly add volumePermissions image as it's often required by Bitnami common
volumePermissions:
  enabled: true # Assume enabled if volumePermissions section is expected
  image:
    # Use a known structure, paths will be overridden if detected
    registry: {TARGET_REGISTRY_PLACEHOLDER}/dockerio 
    repository: bitnami/os-shell
    tag: "12-debian-12" # Use a specific, valid tag
    pullPolicy: IfNotPresent
"""

# VALUES_TEMPLATE_STANDARD_MAP: For charts generally using map-based image definitions
# Keep VERY minimal, avoid adding keys not universally present in schemas.
VALUES_TEMPLATE_STANDARD_MAP = """
global: {} # Assume global might exist but add no specific keys

# Service structure required by many common libraries
# Use nested map structure for ports
service:
  main: # Common primary service name
    ports:
      http: # Common port name
        enabled: true
        port: 80

# Assume image map exists, provide SemVer tag BUT NOT REPOSITORY
image:
  # repository: placeholderRepo # REMOVED: Avoid forcing this key
  tag: "1.0.0" # Use valid SemVer default
  pullPolicy: IfNotPresent
"""

# VALUES_TEMPLATE_STANDARD_STRING: For charts using simple string image definitions
# Keep VERY minimal.
VALUES_TEMPLATE_STANDARD_STRING = f"""
global: {{}}

# Provide basic service structure, adjust if needed
service:
  type: ClusterIP
  port: 80

# Simple image string, tag needs to be valid
image: {TARGET_REGISTRY_PLACEHOLDER}/placeholderRepo:1.0.0 # Use valid SemVer
pullPolicy: IfNotPresent
"""

# VALUES_TEMPLATE_DEFAULT: Fallback - ABSOLUTELY MINIMAL.
# Rely on chart defaults as much as possible.
# Make this completely empty to avoid most schema errors.
VALUES_TEMPLATE_DEFAULT = """{}"""

# Add to the top with other constants
CLASSIFICATION_STATS_FILE = TEST_OUTPUT_DIR / "classification-stats.json"

# Add new global variable for tracking
classification_stats = {
    "counts": defaultdict(int),
    "successes": defaultdict(int),
    "failures": defaultdict(int),
    "details": defaultdict(list),
}

# Regex for extracting version from chart filenames like 'chart-name-1.2.3.tgz'
CHART_VERSION_REGEX = re.compile(
    "-((\\d+\\.){2}\\d+.*?)$"
)  # Ignore case not needed here

DEFAULT_TEST_KUBE_VERSION = "1.31.0"  # Define the default K8s version for tests


@dataclass
class TestResult:
    chart_name: str
    chart_path: Path
    classification: str
    status: str  # e.g., SUCCESS, TEMPLATE_ERROR, OVERRIDE_ERROR, UNKNOWN_ERROR
    category: str  # Error category if status is an error
    details: str  # Detailed error message
    override_duration: float
    validation_duration: float


def categorize_error(error_msg: str) -> str:
    """Categorize an error message based on keywords."""
    if not error_msg:
        return "UNKNOWN_ERROR"

    error_msg_lower = error_msg.lower()

    # Ordered by specificity / common patterns
    # First check for Kubernetes version errors
    if "kubeversion" in error_msg_lower and "incompatible" in error_msg_lower:
        return "KUBERNETES_VERSION_ERROR"
    if "required value" in error_msg_lower or "required field" in error_msg_lower:
        return "REQUIRED_VALUE_ERROR"
    if "failed to parse" in error_msg_lower and (
        "yaml:" in error_msg_lower or "json:" in error_msg_lower
    ):
        return "YAML_ERROR"
    if "invalid yaml syntax" in error_msg_lower:
        return "YAML_ERROR"
    if "schema(s)" in error_msg_lower and (
        "don't meet the specifications" in error_msg_lower
        or "validation failed" in error_msg_lower
    ):
        return "SCHEMA_ERROR"
    if "wrong type for value" in error_msg_lower:
        return "COALESCE_ERROR"
    if (
        "coalesce.go" in error_msg_lower
        and "warning: destination for" in error_msg_lower
    ):
        return "COALESCE_ERROR"
    if "required" in error_msg_lower and (
        "is required" in error_msg_lower
        or "cannot be empty" in error_msg_lower
        or "must be set" in error_msg_lower
    ):
        return "REQUIRED_VALUE_ERROR"
    if "execution error at" in error_msg_lower and (
        "notes.txt" in error_msg_lower or "validations.yaml" in error_msg_lower
    ):
        return "REQUIRED_VALUE_ERROR"
    if "library charts are not installable" in error_msg_lower:
        return "LIBRARY_ERROR"
    if "chart is deprecated" in error_msg_lower:
        return "DEPRECATED_ERROR"
    if "timeout expired" in error_msg_lower:
        return "TIMEOUT_ERROR"
    if "template" in error_msg_lower and (
        "error" in error_msg_lower or "failed" in error_msg_lower
    ):
        return "TEMPLATE_ERROR"
    if "error" in error_msg_lower or "failed" in error_msg_lower:
        return "UNKNOWN_ERROR"

    return "UNKNOWN_ERROR"


def test_chart(chart: str, chart_dir: Path) -> bool:
    """Test a chart's template rendering."""
    chart_name = os.path.basename(chart)
    chart_path = chart_dir / chart_name
    stdout_file = TEST_OUTPUT_DIR / f"{chart_name}-analysis-stdout.txt"
    stderr_file = TEST_OUTPUT_DIR / f"{chart_name}-analysis-stderr.txt"
    default_values_file = Path(__file__).parent / "lib" / "default-values.yaml"

    print(f"Testing chart template: {chart}")

    try:
        # Don't capture the result since we only care about success/failure
        subprocess.run(
            ["helm", "template", str(chart_path), "-f", str(default_values_file)],
            stdout=open(stdout_file, "w"),
            stderr=open(stderr_file, "w"),
            check=True,
        )
        return True
    except subprocess.CalledProcessError:
        log_error(
            ANALYSIS_RESULTS_FILE,
            f"{chart}: Template rendering failed",
            stderr_file,
            stdout_file,
        )
        return False


def update_classification_stats(chart_name: str, classification: str, success: bool):
    """Update classification statistics."""
    classification_stats["counts"][classification] += 1
    if success:
        classification_stats["successes"][classification] += 1
    else:
        classification_stats["failures"][classification] += 1
    classification_stats["details"][classification].append(
        {"chart": chart_name, "success": success}
    )


def save_classification_stats():
    """Save classification statistics to file."""
    stats_data = {
        "counts": dict(classification_stats["counts"]),
        "successes": dict(classification_stats["successes"]),
        "failures": dict(classification_stats["failures"]),
        "details": dict(classification_stats["details"]),
        "success_rates": {
            cls: (classification_stats["successes"][cls] / count if count > 0 else 0)
            for cls, count in classification_stats["counts"].items()
        },
    }

    with open(CLASSIFICATION_STATS_FILE, "w") as f:
        json.dump(stats_data, f, indent=2)

    # Print summary to console
    print("\nClassification Statistics:")
    print("-" * 40)
    for cls in stats_data["counts"].keys():
        success_rate = stats_data["success_rates"][cls] * 100
        print(f"{cls}:")
        print(f"  Total: {stats_data['counts'][cls]}")
        print(f"  Success Rate: {success_rate:.1f}%")
    print("-" * 40)


async def test_chart_override(chart_info, target_registry, irr_binary, session, args):
    """Test chart override generation.

    Args:
        chart_info: Tuple of (chart_name, chart_path)
        target_registry: Target registry URL
        irr_binary: Path to irr binary
        session: Unused session parameter (kept for compatibility)
        args: Command line arguments
    """
    chart_name, chart_path = chart_info
    output_file = TEST_OUTPUT_DIR / f"{chart_name}-values.yaml"
    debug_log_file = TEST_OUTPUT_DIR / f"{chart_name}-override-debug.log"

    # --- Clean up any existing output YAML file for this chart ---
    if output_file.exists():
        output_file.unlink()

    # --- Initialize variables ---
    classification = "UNKNOWN"
    override_duration = 0
    result = None

    # Create debug log file
    debug_log_file.parent.mkdir(parents=True, exist_ok=True)
    debug_log = open(debug_log_file, "w")

    def log_debug(msg):
        """Helper to log debug messages selectively."""
        # Always write to log file
        debug_log.write(f"{msg}\n")
        debug_log.flush()

        # Only print to console if debug flag is enabled
        if args.debug:
            # For large JSON/YAML content, truncate console output
            if len(msg) > 200 and ("JSON" in msg or "YAML" in msg):
                truncated = msg[:197] + "..."
                print(f"  DEBUG: {truncated} (full content in log file)")
            else:
                print(f"  DEBUG: {msg}")

    try:
        log_debug(f"Generating overrides for chart {chart_name} from {chart_path}")

        # --- Chart Path Handling ---
        chart_path = Path(chart_path)
        if not chart_path.exists():
            raise FileNotFoundError(f"Chart path does not exist: {chart_path}")

        # If it's a .tgz file, use it directly
        if chart_path.suffix == ".tgz":
            log_debug(f"Using chart archive: {chart_path}")
        else:
            # For directory paths, ensure it's a valid chart directory
            if not (chart_path / "Chart.yaml").exists():
                raise ValueError(
                    f"Invalid chart directory: {chart_path} - Chart.yaml not found"
                )
            log_debug(f"Using chart directory: {chart_path}")

        # --- Override Generation ---
        log_debug(f"Generating overrides for {chart_name}...")
        start_time = time.time()
        # --- Command Construction ---
        override_cmd = [
            str(irr_binary),
            "override",
            "--chart-path",
            str(chart_path),
            "--output-file",
            str(output_file),
            "--target-registry",
            target_registry,
            "--source-registries",
            args.source_registries,
            "--no-validate",
        ]

        # Add optional parameters if specified
        if hasattr(args, "target_tag") and args.target_tag:
            override_cmd.extend(["--target-tag", args.target_tag])
        if hasattr(args, "target_repository") and args.target_repository:
            override_cmd.extend(["--target-repository", args.target_repository])

        log_debug(f"Running command: {' '.join(override_cmd)}")

        # --- Execute Command ---
        try:
            result = subprocess.run(
                override_cmd,
                capture_output=True,
                text=True,
                timeout=120,
            )
            if result.returncode != 0:
                error_msg = f"Command failed with exit code {result.returncode}: {result.stderr}"
                log_debug(f"Error: {error_msg}")
                return TestResult(
                    chart_name,
                    chart_path,
                    classification,
                    "OVERRIDE_ERROR",
                    "OVERRIDE_ERROR",
                    error_msg,
                    0,
                    0,
                )
        except subprocess.TimeoutExpired:
            error_msg = "Command timed out after 120 seconds"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "TIMEOUT_ERROR",
                "TIMEOUT_ERROR",
                error_msg,
                0,
                0,
            )
        except Exception as e:
            error_msg = f"Error executing command: {e}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "OVERRIDE_ERROR",
                "OVERRIDE_ERROR",
                error_msg,
                0,
                0,
            )

        # --- Verify Output ---
        if not output_file.exists():
            error_msg = f"Expected output file not found: {output_file}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "OVERRIDE_ERROR",
                "OVERRIDE_ERROR",
                error_msg,
                0,
                0,
            )

        # --- Parse Output ---
        try:
            with open(output_file) as f:
                override_yaml = yaml.safe_load(f)
            log_debug(f"Override YAML content: {json.dumps(override_yaml, indent=2)}")
        except Exception as e:
            error_msg = f"Error parsing override YAML: {e}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "TEMPLATE_ERROR",
                "TEMPLATE_ERROR",
                error_msg,
                0,
                0,
            )

        # --- Validate Output ---
        if not isinstance(override_yaml, dict):
            error_msg = "Override YAML is not a dictionary"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "TEMPLATE_ERROR",
                "TEMPLATE_ERROR",
                error_msg,
                0,
                0,
            )

        # --- Success ---
        override_duration = time.time() - start_time
        return TestResult(
            chart_name,
            chart_path,
            classification,
            "SUCCESS",
            "SUCCESS",
            "Override generated successfully",
            override_duration,
            0,
        )

    except Exception as e:
        error_msg = f"Unexpected error processing chart {chart_name}: {str(e)}"
        log_debug(f"Error: {error_msg}")
        import traceback

        traceback.print_exc(file=debug_log)
        return TestResult(
            chart_name,
            chart_path,
            classification,
            "UNKNOWN_ERROR",
            "UNKNOWN_ERROR",
            error_msg,
            0,
            0,
        )

    finally:
        # Close debug log file
        debug_log.close()


# Define the new function based on test_chart_override
async def test_chart_override_with_internal_validate(chart_info, target_registry, irr_binary, session, args):
    """Test chart override generation, *including* internal validation."""
    chart_name, chart_path = chart_info
    # Use a slightly different output file name for clarity, though it might be overwritten
    output_file = TEST_OUTPUT_DIR / f"{chart_name}-values-internal-validate.yaml"
    # Use a different debug log file name
    debug_log_file = TEST_OUTPUT_DIR / f"{chart_name}-override-internal-validate-debug.log"

    # --- Clean up any existing output YAML file for this chart --- #
    if output_file.exists():
        output_file.unlink()

    # --- Initialize variables --- #
    classification = "UNKNOWN" # Will be determined later if needed
    override_duration = 0
    result = None

    # Create debug log file
    debug_log_file.parent.mkdir(parents=True, exist_ok=True)
    debug_log = open(debug_log_file, "w")

    def log_debug(msg):
        """Helper to log debug messages selectively."""
        debug_log.write(f"{msg}\n")
        debug_log.flush()
        if args.debug:
            if len(msg) > 200 and ("JSON" in msg or "YAML" in msg):
                truncated = msg[:197] + "..."
                print(f"  DEBUG: {truncated} (full content in log file)")
            else:
                print(f"  DEBUG: {msg}")

    try:
        log_debug(f"Generating overrides WITH internal validation for chart {chart_name} from {chart_path}")

        # --- Chart Path Handling --- #
        chart_path = Path(chart_path)
        if not chart_path.exists():
            raise FileNotFoundError(f"Chart path does not exist: {chart_path}")

        if chart_path.suffix == ".tgz":
            log_debug(f"Using chart archive: {chart_path}")
        else:
            if not (chart_path / "Chart.yaml").exists():
                raise ValueError(
                    f"Invalid chart directory: {chart_path} - Chart.yaml not found"
                )
            log_debug(f"Using chart directory: {chart_path}")

        # --- Override Generation --- #
        log_debug(f"Generating overrides for {chart_name} (internal validation enabled)...")
        start_time = time.time()
        # --- Command Construction (NO --no-validate flag) --- #
        override_cmd = [
            str(irr_binary),
            "override",
            "--chart-path",
            str(chart_path),
            "--output-file",
            str(output_file),
            "--target-registry",
            target_registry,
            "--source-registries",
            args.source_registries,
            # NO "--no-validate" flag here
        ]

        if hasattr(args, "target_tag") and args.target_tag:
            override_cmd.extend(["--target-tag", args.target_tag])
        if hasattr(args, "target_repository") and args.target_repository:
            override_cmd.extend(["--target-repository", args.target_repository])
        # Add debug flag if specified in script args
        if args.debug:
            override_cmd.append("--debug")

        log_debug(f"Running command: {' '.join(override_cmd)}")

        # --- Execute Command --- #
        try:
            result = subprocess.run(
                override_cmd,
                capture_output=True,
                text=True,
                timeout=args.timeout, # Use configured timeout
            )
            override_duration = time.time() - start_time
            # --- IMPORTANT: Handle expected validation failures --- #
            # If the command fails (non-zero exit code), categorize the error.
            # We EXPECT failures here due to internal validation.
            if result.returncode != 0:
                # Use stderr first, then stdout if stderr is empty
                error_msg_detail = result.stderr.strip() or result.stdout.strip()
                full_error_msg = f"Command failed with exit code {result.returncode}: {error_msg_detail}"
                error_category = categorize_error(error_msg_detail)
                log_debug(f"Expected failure (Internal Validation): Code {result.returncode}, Category: {error_category}, Msg: {full_error_msg[:200]}...")

                # Return a specific status indicating internal validation failure
                return TestResult(
                    chart_name,
                    chart_path,
                    classification,
                    "OVERRIDE_INTERNAL_VALIDATION_FAILURE", # Specific status
                    error_category, # Categorize the underlying Helm error
                    full_error_msg,
                    override_duration,
                    0, # No separate validation duration for this op
                )
            # If return code is 0, it means override AND internal validation succeeded
            else:
                 log_debug(f"Override and internal validation SUCCEEDED unexpectedly for {chart_name} (Code {result.returncode}).")
                 # Fall through to check output file existence etc.

        except subprocess.TimeoutExpired:
            override_duration = time.time() - start_time
            error_msg = f"Command (with internal validation) timed out after {args.timeout} seconds"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "TIMEOUT_ERROR",
                "TIMEOUT_ERROR",
                error_msg,
                override_duration,
                0,
            )
        except Exception as e:
            override_duration = time.time() - start_time
            error_msg = f"Error executing command (with internal validation): {e}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "OVERRIDE_ERROR", # Generic override error for unexpected exceptions
                "OVERRIDE_ERROR",
                error_msg,
                override_duration,
                0,
            )

        # --- Verify Output File (even on unexpected success) --- #
        if not output_file.exists():
            # This could happen if the command succeeded (code 0) but somehow didn't write the file
            error_msg = f"Command SUCCEEDED (Code 0) but expected output file not found: {output_file}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "OVERRIDE_ERROR", # Treat as an error
                "OVERRIDE_ERROR",
                error_msg,
                override_duration,
                0,
            )

        # --- Parse Output (only if command succeeded, which is rare/unexpected here) --- #
        try:
            with open(output_file) as f:
                override_yaml = yaml.safe_load(f)
            log_debug(f"Override YAML content (Internal Validation Success): {json.dumps(override_yaml, indent=2)}")
        except Exception as e:
            error_msg = f"Command SUCCEEDED (Code 0) but error parsing override YAML: {e}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "YAML_ERROR", # Categorize as YAML error
                "YAML_ERROR",
                error_msg,
                override_duration,
                0,
            )

        # --- Validate Output Structure (only if command succeeded) --- #
        if not isinstance(override_yaml, dict):
            error_msg = "Command SUCCEEDED (Code 0) but override YAML is not a dictionary"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name,
                chart_path,
                classification,
                "YAML_ERROR",
                "YAML_ERROR",
                error_msg,
                override_duration,
                0,
            )

        # --- Unexpected Success --- #
        # If we reach here, the override AND the internal validation passed.
        # This is unusual but technically a success for this specific operation.
        return TestResult(
            chart_name,
            chart_path,
            classification,
            "SUCCESS", # Mark as success for this specific test run
            "SUCCESS",
            "Override generated and internal validation passed successfully",
            override_duration,
            0,
        )

    except Exception as e:
        error_msg = f"Unexpected error processing chart {chart_name} (with internal validation): {str(e)}"
        log_debug(f"Error: {error_msg}")
        import traceback
        traceback.print_exc(file=debug_log)
        return TestResult(
            chart_name,
            chart_path,
            classification,
            "UNKNOWN_ERROR",
            "UNKNOWN_ERROR",
            error_msg,
            0, # Duration might be unknown here
            0,
        )

    finally:
        # Close debug log file
        debug_log.close()


async def test_chart_inspect(chart_info, target_registry, irr_binary, session, args):
    """Test the inspect operation for a single chart."""
    # target_registry is ignored in this function but kept for signature consistency
    chart_name, chart_path = chart_info
    # Define output and log files specific to inspect
    stdout_file = DETAILED_LOGS_DIR / f"{chart_name}-inspect-stdout.log"
    stderr_file = DETAILED_LOGS_DIR / f"{chart_name}-inspect-stderr.log"
    detailed_log_file = DETAILED_LOGS_DIR / f"{chart_name}-inspect.log"

    stdout_file.parent.mkdir(parents=True, exist_ok=True)

    def log_debug(msg):
        with open(detailed_log_file, "a", encoding="utf-8") as f:
            f.write(f"[{datetime.now()}] DEBUG: {msg}\n")
        if args.debug:
            print(f"  DEBUG: {msg}")

    log_debug(f"Starting inspect for chart: {chart_name} ({chart_path})")

    start_time = time.monotonic()
    validation_duration = 0 # Not applicable here, but keep consistent structure

    try:
        # --- Chart Path Handling --- #
        chart_path = Path(chart_path)
        if not chart_path.exists():
            raise FileNotFoundError(f"Chart path does not exist: {chart_path}")
        log_debug(f"Using chart path: {chart_path}")

        # --- Construct Inspect Command --- #
        inspect_cmd = [
            str(irr_binary),
            "inspect",
            "--chart-path",
            str(chart_path),
            "--output-format", "json", # Request JSON output
            # Add other relevant inspect flags if needed, e.g., --output-format
            # Defaulting to YAML output
        ]
        # Add debug flag if specified in script args
        if args.debug:
            inspect_cmd.append("--debug")

        log_debug(f"Running inspect command: {' '.join(inspect_cmd)}")

        # --- Execute Inspect Command --- #
        process = await asyncio.create_subprocess_exec(
            *inspect_cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout_bytes, stderr_bytes = await asyncio.wait_for(
            process.communicate(),
            timeout=args.timeout, # Use configured timeout
        )
        stdout = stdout_bytes.decode("utf-8", errors="ignore")
        stderr = stderr_bytes.decode("utf-8", errors="ignore")
        return_code = process.returncode

        with open(stdout_file, "w", encoding="utf-8") as f:
            f.write(stdout)
        with open(stderr_file, "w", encoding="utf-8") as f:
            f.write(stderr)

        end_time = time.monotonic()
        inspect_duration = end_time - start_time # Use a specific duration variable
        log_debug(f"Inspect command finished with code {return_code} in {inspect_duration:.2f}s")

        # --- Process Results --- #
        if return_code == 0:
            # Attempt to parse YAML output for basic validation
            try:
                yaml.safe_load(stdout)
                log_debug("Inspect successful and output parsed as YAML.")
                # Classification is not directly available/relevant here like in validate
                classification = "UNKNOWN" # Or determine if needed
                return TestResult(
                    chart_name=chart_name,
                    chart_path=chart_path,
                    classification=classification,
                    status="SUCCESS",
                    category="",
                    details="Inspect command successful and output is valid YAML.",
                    override_duration=0, # Not applicable
                    validation_duration=inspect_duration, # Use inspect duration here
                )
            except yaml.YAMLError as e:
                error_details = f"Inspect command succeeded (code 0) but output is not valid YAML: {e}\nOutput:\n{stdout[:500]}..."
                log_debug(f"Error: {error_details}")
                return TestResult(
                    chart_name=chart_name,
                    chart_path=chart_path,
                    classification="UNKNOWN",
                    status="INSPECT_OUTPUT_ERROR",
                    category="YAML_ERROR",
                    details=error_details,
                    override_duration=0,
                    validation_duration=inspect_duration,
                )
        else:
            error_details = stderr.strip() or stdout.strip()
            error_category = categorize_error(error_details)
            log_debug(f"Inspect failed. Category: {error_category}, Details: {error_details[:500]}...")
            return TestResult(
                chart_name=chart_name,
                chart_path=chart_path,
                classification="UNKNOWN",
                status="INSPECT_ERROR", # Specific status for inspect failure
                category=error_category,
                details=error_details,
                override_duration=0,
                validation_duration=inspect_duration,
            )

    except asyncio.TimeoutError:
        end_time = time.monotonic()
        inspect_duration = end_time - start_time
        log_debug(f"Inspect command timed out after {inspect_duration:.2f}s")
        return TestResult(
            chart_name=chart_name,
            chart_path=chart_path,
            classification="UNKNOWN",
            status="TIMEOUT_ERROR",
            category="TIMEOUT_ERROR",
            details=f"Inspect timed out after {args.timeout} seconds",
            override_duration=0,
            validation_duration=inspect_duration,
        )
    except Exception as e:
        end_time = time.monotonic()
        inspect_duration = end_time - start_time
        error_details = f"Unexpected error during inspect: {e}"
        log_debug(error_details)
        import traceback
        with open(detailed_log_file, "a", encoding="utf-8") as f:
             traceback.print_exc(file=f)
        return TestResult(
            chart_name=chart_name,
            chart_path=chart_path,
            classification="UNKNOWN",
            status="UNKNOWN_ERROR",
            category="UNKNOWN_ERROR",
            details=error_details,
            override_duration=0,
            validation_duration=inspect_duration,
        )


async def test_chart_validate(chart_info, target_registry, irr_binary, session, args):
    """Test the validation (template) process for a single chart."""
    chart_name = chart_info["chart_name"]
    chart_path = chart_info["chart_path"]
    classification = chart_info["classification"]
    detailed_log_file = DETAILED_LOGS_DIR / f"{chart_name}-validate.log"
    stdout_file = DETAILED_LOGS_DIR / f"{chart_name}-validate-stdout.log"
    stderr_file = DETAILED_LOGS_DIR / f"{chart_name}-validate-stderr.log"

    def log_debug(msg):
        with open(detailed_log_file, "a", encoding="utf-8") as f:
            f.write(f"[{datetime.now()}] DEBUG: {msg}\n")

    log_debug(f"Starting validation for chart: {chart_name} ({chart_path})")
    log_debug(f"Classification: {classification}")

    # Create a temporary directory for validation outputs
    with tempfile.TemporaryDirectory(
        prefix=f"irr-test-validate-{chart_name.replace('/', '_')}-",
        dir=TEST_OUTPUT_DIR,
    ) as temp_dir:
        temp_path = Path(temp_dir)
        override_file_path = chart_info.get("override_file_path")

        # --- 1. Prepare Override File (Copy from previous step or create empty) ---
        if override_file_path and override_file_path.exists():
            shutil.copy(override_file_path, temp_path / "override.yaml")
            log_debug(f"Using override file from previous step: {override_file_path}")
        else:
            # Create an empty override file if none exists (shouldn't happen often)
            (temp_path / "override.yaml").touch()
            log_debug("Override file not found from previous step, creating empty.")

        # --- 2. Prepare Values File ---
        values_content = get_values_content(classification, target_registry)
        with open(temp_path / "values.yaml", "w", encoding="utf-8") as f:
            f.write(values_content)
        log_debug(f"Using values file for classification '{classification}'")

        # Ensure chart_path is a proper Path object
        if not isinstance(chart_path, Path):
            chart_path = Path(chart_path)

        # Ensure the chart path exists and is valid
        log_debug(f"Validating chart path exists: {chart_path}")
        if not chart_path.exists():
            error_msg = f"Chart path does not exist: {chart_path}"
            log_debug(f"Error: {error_msg}")
            return TestResult(
                chart_name=chart_name,
                chart_path=chart_path,
                classification=classification,
                status="SETUP_ERROR",
                category="SETUP_ERROR",
                details=error_msg,
                override_duration=chart_info.get("override_duration", 0),
                validation_duration=0,
            )

        # --- 3. Construct Validation Command ---
        validate_cmd = [
            str(irr_binary),
            "validate",
            "--chart-path",
            chart_path.absolute().as_posix(),  # Use absolute posix path to avoid escaping issues
            "--values",
            Path(override_file_path)
            .absolute()
            .as_posix(),  # Use absolute posix path to avoid escaping issues
        ]

        # Add release name only if needed
        if hasattr(args, "release_name") and args.release_name:
            validate_cmd.extend(["--release-name", args.release_name])
        else:
            # Default release name based on chart
            release_name = chart_name.split("/")[-1]
            # Ensure no spaces in release name
            release_name = release_name.replace(" ", "-")
            validate_cmd.extend(["--release-name", f"release-{release_name}"])

        # Add debug flag if specified
        if args.debug:
            validate_cmd.append("--debug")

        log_debug(f"Running validation command: {' '.join(validate_cmd)}")

        # --- 4. Execute Validation Command ---
        start_time = time.monotonic()
        try:
            process = await asyncio.create_subprocess_exec(
                *validate_cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=temp_path,  # Run in temp dir to isolate
            )
            stdout_bytes, stderr_bytes = await asyncio.wait_for(
                process.communicate(),
                timeout=args.timeout * 2,  # Longer timeout for validate
            )
            stdout = stdout_bytes.decode("utf-8", errors="ignore")
            stderr = stderr_bytes.decode("utf-8", errors="ignore")
            return_code = process.returncode

            with open(stdout_file, "w", encoding="utf-8") as f:
                f.write(stdout)
            with open(stderr_file, "w", encoding="utf-8") as f:
                f.write(stderr)

            end_time = time.monotonic()
            validation_duration = end_time - start_time
            log_debug(
                f"Validation command finished with code {return_code} in {validation_duration:.2f}s"
            )

            if return_code == 0:
                log_debug("Validation successful.")
                update_classification_stats(chart_name, classification, success=True)
                return TestResult(
                    chart_name=chart_name,
                    chart_path=chart_path,
                    classification=classification,
                    status="SUCCESS",
                    category="",
                    details="",
                    override_duration=chart_info.get("override_duration", 0),
                    validation_duration=validation_duration,
                )
            else:
                # Get complete error details without truncation
                error_details = stderr.strip() or stdout.strip()

                # For exit code 18 (path-related errors), include the full command in the error details
                if "exit code 18" in error_details:
                    log_debug(
                        f"Path-related error (exit code 18) detected with chart {chart_path}"
                    )
                    error_details = f"Command failed with exit code 18. Full command: {' '.join(validate_cmd)}\nOriginal error: {error_details}"

                error_category = categorize_error(error_details)
                log_debug(
                    f"Validation failed. Category: {error_category}, Details: {error_details[:500]}..."
                )

                # --- 5. Fallback Logic for KUBERNETES_VERSION_ERROR ---
                if error_category == "KUBERNETES_VERSION_ERROR":
                    log_debug(
                        "Kubernetes version error detected. Attempting fallback with direct Helm template."
                    )
                    k8s_versions_to_try = [
                        "1.28.0",
                        "1.27.0",
                        "1.25.0",
                        "1.23.0",
                    ]  # Example older versions

                    for k8s_version in k8s_versions_to_try:
                        log_debug(
                            f"Retrying with Helm template and K8s version: {k8s_version}"
                        )
                        helm_cmd = [
                            "helm",
                            "template",
                            f"{chart_name.split('/')[-1]}-test",
                            str(chart_path),
                            "--values",
                            str(override_file_path),
                            "--kube-version",
                            k8s_version,  # Use --kube-version in fallback
                            # RETAIN --set Capabilities.* in fallback for charts that strictly need it
                            "--set",
                            f"Capabilities.KubeVersion.Version=v{k8s_version}",
                            "--set",
                            f"Capabilities.KubeVersion.Major={k8s_version.split('.')[0]}",
                            "--set",
                            f"Capabilities.KubeVersion.Minor={k8s_version.split('.')[1]}",
                        ]
                        log_debug(f"Fallback Helm command: {' '.join(helm_cmd)}")
                        try:
                            helm_process = await asyncio.create_subprocess_exec(
                                *helm_cmd,
                                stdout=asyncio.subprocess.PIPE,
                                stderr=asyncio.subprocess.PIPE,
                                cwd=temp_path,
                            )
                            (
                                helm_stdout_bytes,
                                helm_stderr_bytes,
                            ) = await asyncio.wait_for(
                                helm_process.communicate(), timeout=args.timeout
                            )
                            # We only need stderr for error checking
                            # helm_stdout = helm_stdout_bytes.decode("utf-8", errors="ignore")  # Unused variable
                            helm_stderr = helm_stderr_bytes.decode(
                                "utf-8", errors="ignore"
                            )

                            if helm_process.returncode == 0:
                                log_debug(
                                    f"Fallback successful with K8s version {k8s_version}."
                                )
                                # Log success but still categorize original failure
                                update_classification_stats(
                                    chart_name, classification, success=False
                                )  # Mark original as failure
                                return TestResult(
                                    chart_name=chart_name,
                                    chart_path=chart_path,
                                    classification=classification,
                                    status="SUCCESS_FALLBACK",  # Indicate success via fallback
                                    category=error_category,  # Keep original category
                                    details=f"Original failure ({error_category}): {error_details[:200]}... Succeeded with fallback K8s version {k8s_version}",
                                    override_duration=chart_info.get(
                                        "override_duration", 0
                                    ),
                                    validation_duration=validation_duration,  # Use original duration
                                )
                            else:
                                log_debug(
                                    f"Fallback failed with K8s version {k8s_version}. Error: {helm_stderr.strip()[:200]}..."
                                )
                                # Continue to next version if this one failed

                        except asyncio.TimeoutError:
                            log_debug(
                                f"Fallback Helm template timed out for version {k8s_version}."
                            )
                            # Continue to next version
                        except Exception as e:
                            log_debug(
                                f"Exception during fallback Helm template for version {k8s_version}: {e}"
                            )
                            # Continue to next version

                    # If all fallbacks fail, return the original error
                    log_debug("All fallback attempts failed. Reporting original error.")
                    update_classification_stats(
                        chart_name, classification, success=False
                    )
                    return TestResult(
                        chart_name=chart_name,
                        chart_path=chart_path,
                        classification=classification,
                        status="TEMPLATE_ERROR",  # Report original error type
                        category=error_category,
                        details=error_details,
                        override_duration=chart_info.get("override_duration", 0),
                        validation_duration=validation_duration,
                    )

                # --- 6. Handle Other Errors (Non-K8s Version) ---
                else:
                    log_debug("Validation failed with non-K8s version error.")
                    update_classification_stats(
                        chart_name, classification, success=False
                    )
                    return TestResult(
                        chart_name=chart_name,
                        chart_path=chart_path,
                        classification=classification,
                        status="TEMPLATE_ERROR",  # Or more specific if possible
                        category=error_category,
                        details=error_details,
                        override_duration=chart_info.get("override_duration", 0),
                        validation_duration=validation_duration,
                    )

        except asyncio.TimeoutError:
            end_time = time.monotonic()
            validation_duration = end_time - start_time
            log_debug(f"Validation command timed out after {validation_duration:.2f}s")
            update_classification_stats(chart_name, classification, success=False)
            return TestResult(
                chart_name=chart_name,
                chart_path=chart_path,
                classification=classification,
                status="TIMEOUT_ERROR",
                category="TIMEOUT_ERROR",
                details=f"Validation timed out after {args.timeout * 2} seconds",
                override_duration=chart_info.get("override_duration", 0),
                validation_duration=validation_duration,
            )
        except Exception as e:
            end_time = time.monotonic()
            validation_duration = end_time - start_time
            error_details = f"Unexpected error during validation: {e}"
            log_debug(error_details)
            update_classification_stats(chart_name, classification, success=False)
            return TestResult(
                chart_name=chart_name,
                chart_path=chart_path,
                classification=classification,
                status="UNKNOWN_ERROR",
                category="UNKNOWN_ERROR",
                details=error_details,
                override_duration=chart_info.get("override_duration", 0),
                validation_duration=validation_duration,
            )


# --- Helper Functions for Subchart Analysis --- #

# Define known image paths for common K8s kinds
KNOWN_IMAGE_PATHS = {
    "Pod": [
        "spec.containers[*].image",
        "spec.initContainers[*].image",
        "spec.ephemeralContainers[*].image",
    ],
    "Deployment": [
        "spec.template.spec.containers[*].image",
        "spec.template.spec.initContainers[*].image",
        "spec.template.spec.ephemeralContainers[*].image",
    ],
    "StatefulSet": [
        "spec.template.spec.containers[*].image",
        "spec.template.spec.initContainers[*].image",
        "spec.template.spec.ephemeralContainers[*].image",
    ],
    "DaemonSet": [
        "spec.template.spec.containers[*].image",
        "spec.template.spec.initContainers[*].image",
        "spec.template.spec.ephemeralContainers[*].image",
    ],
    "Job": [
        "spec.template.spec.containers[*].image",
        "spec.template.spec.initContainers[*].image",
        "spec.template.spec.ephemeralContainers[*].image",
    ],
    "CronJob": [
        "spec.jobTemplate.spec.template.spec.containers[*].image",
        "spec.jobTemplate.spec.template.spec.initContainers[*].image",
        "spec.jobTemplate.spec.template.spec.ephemeralContainers[*].image",
    ],
    # Add others like ReplicaSet, ReplicationController if needed
}

def _safe_get(data, path_parts, found_images):
    """Recursively navigate dict/list structure and extract images."""
    current = data
    for i, part in enumerate(path_parts):
        if isinstance(current, dict):
            if part == "[*]":
                 # This part should handle list iteration, called from the outer loop
                 # This specific function expects a direct key or index
                 return # Should not happen if called correctly
            current = current.get(part)
            if current is None:
                return # Path doesn't exist
        elif isinstance(current, list) and part == "[*]":
            remaining_path = path_parts[i+1:]
            for item in current:
                 # Ensure item is a dict before recursing if path expects keys
                 if isinstance(item, dict):
                     _safe_get(item, remaining_path, found_images)
                 # else: item is not a dict, cannot traverse further down this path branch
            return # Handled list iteration, stop processing this branch
        else:
            # Trying to access dict key on non-dict, or list index mismatch etc.
            return # Invalid path for this structure

    # If we reached the end of the path, check if it's an image string
    if isinstance(current, str) and current: # Check if non-empty string
        # Basic check to avoid adding non-image-like strings (optional refinement)
        # Heuristic: Must contain either / or : to be considered image-like
        # Avoids matching simple strings like "true", "enabled", etc.
        if '/' in current or ':' in current:
             found_images.add(current)

def extract_images_from_doc(doc: dict) -> Set[str]:
    """Extracts image strings from a parsed K8s manifest document."""
    found_images: Set[str] = set()
    kind = doc.get("kind")

    if not isinstance(doc, dict):
        return found_images # Skip if doc is not a dictionary

    # Use Kind if available, otherwise use Pod paths as a fallback guess
    # This helps catch images in basic Pod specs even if Kind is missing/different
    paths_to_check = KNOWN_IMAGE_PATHS.get(kind, KNOWN_IMAGE_PATHS.get("Pod", []))

    for path_str in paths_to_check:
        path_parts = path_str.split('.')
        _safe_get(doc, path_parts, found_images)

    return found_images


async def test_chart_subchart_analysis(chart_info, target_registry, irr_binary, session, args):
    """Runs inspect, checks for subchart discrepancy, and gathers data if found."""
    chart_name, chart_path = chart_info
    chart_path_obj = Path(chart_path) # Ensure Path object
    # Define output and log files specific to subchart analysis
    inspect_stdout_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-inspect-stdout.log"
    inspect_stderr_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-inspect-stderr.log"
    template_stdout_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-template-stdout.log"
    template_stderr_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-template-stderr.log"
    # show_values_stdout_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-show_values-stdout.log" # Keep for now, might remove later
    # show_values_stderr_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-show_values-stderr.log" # Keep for now, might remove later
    detailed_log_file = DETAILED_LOGS_DIR / f"{chart_name}-subchart-analysis.log"

    # Ensure logs dir exists
    detailed_log_file.parent.mkdir(parents=True, exist_ok=True)

    # Use a lock for writing to the shared JSON results file
    # results_lock = asyncio.Lock() # Will uncomment when writing results
    results_lock = asyncio.Lock() # Defined lock for file writing
    subchart_results_file = TEST_OUTPUT_DIR / "subchart_analysis_results.json"

    def log_debug(msg):
        with open(detailed_log_file, "a", encoding="utf-8") as f:
            f.write(f"[{datetime.now()}] DEBUG: {msg}\n")
        if args.debug:
            # Simple console logging for now
            print(f"  DEBUG [{chart_name}]: {msg}")

    log_debug(f"Starting subchart analysis for: {chart_name} ({chart_path_obj})")
    start_time = time.monotonic()
    analyzer_images_data = {} # Store parsed JSON from inspect

    try:
        # --- 1. Run irr inspect ---
        log_debug("Running initial irr inspect...")
        inspect_cmd = [
            str(irr_binary),
            "inspect",
            "--chart-path",
            str(chart_path_obj),
            "--output-format", "json", # Request JSON output
        ]
        if args.debug:
            inspect_cmd.append("--debug")

        process = await asyncio.create_subprocess_exec(
            *inspect_cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        stdout_bytes, stderr_bytes = await asyncio.wait_for(
            process.communicate(),
            timeout=args.timeout,
        )
        inspect_stdout = stdout_bytes.decode("utf-8", errors="ignore")
        inspect_stderr = stderr_bytes.decode("utf-8", errors="ignore")
        inspect_rc = process.returncode

        # Save inspect output
        with open(inspect_stdout_file, "w", encoding="utf-8") as f: f.write(inspect_stdout)
        with open(inspect_stderr_file, "w", encoding="utf-8") as f: f.write(inspect_stderr)

        if inspect_rc != 0:
             log_debug(f"Initial irr inspect failed with code {inspect_rc}")
             raise RuntimeError(f"irr inspect failed: {inspect_stderr[:500]}")

        # --- 1.5 Parse irr inspect JSON output ---
        try:
            analyzer_images_data = json.loads(inspect_stdout)
            # Extract image strings into a set for easier comparison later
            analyzer_images_set = set()
            if isinstance(analyzer_images_data.get('images'), list):
                for img_info in analyzer_images_data['images']:
                    if isinstance(img_info, dict) and 'image' in img_info:
                        analyzer_images_set.add(img_info['image'])
            log_debug(f"Found {len(analyzer_images_set)} unique images via irr inspect.")
        except json.JSONDecodeError as e:
            log_debug(f"Failed to parse irr inspect JSON output: {e}")
            # Treat as an error for this analysis
            raise RuntimeError(f"Could not parse irr inspect JSON: {e}")


        # --- 2. Check for Discrepancy Warning ---
        # This check remains useful as a secondary indicator, even if we parse JSON now.
        discrepancy_warning_found = False
        analyzer_count_from_warning, template_count_from_warning = 0, 0 # Default counts from warning msg
        warning_line = ""
        # More robust check: search the entire stderr string
        if 'check="subchart_discrepancy"' in inspect_stderr:
            discrepancy_warning_found = True
            # Try to find the specific line for logging/parsing counts
            for line in inspect_stderr.splitlines():
                if 'check="subchart_discrepancy"' in line:
                    warning_line = line
                    log_debug(f"Discrepancy warning found in stderr: {line}")
                    # Attempt to parse counts from the log line (basic parsing)
                    try:
                        # Simple string splitting might be fragile with JSON logs
                        # Consider regex or JSON parsing if needed, but try this first
                        parts = line.split()
                        for part in parts:
                            if part.startswith("analyzer_image_count="):
                                count_str = part.split("=")[1].strip('",')
                                analyzer_count_from_warning = int(count_str)
                            elif part.startswith("template_image_count="):
                                count_str = part.split("=")[1].strip('",')
                                template_count_from_warning = int(count_str)
                    except Exception as e:
                        log_debug(f"Could not parse counts from warning line: {e} - Line: {line}")
                    break # Found the line, no need to check further

        # --- 3. Gather Helm Template Data ---
        log_debug("Gathering helm template output...")
        rendered_yaml = ""
        helm_template_stderr = ""
        helm_template_rc = -1 # Initialize return code

        # Use classification logic and temp values file
        classification = get_chart_classification(chart_path_obj) if chart_path_obj.is_dir() else "UNKNOWN"
        values_content = get_values_content(classification, target_registry)
        temp_values_path = None # Initialize path

        try:
            # Write temp values file
            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False, encoding='utf-8') as temp_values_file:
                temp_values_path = temp_values_file.name
                temp_values_file.write(values_content)
            log_debug(f"Generated temporary values file for helm: {temp_values_path}")

            # Determine release name (reuse logic from validate)
            release_name = chart_name.split("/")[-1].replace(" ", "-")
            helm_template_cmd = [
                "helm",
                "template",
                f"rel-{release_name}", # Use a simple release name
                 str(chart_path_obj),
                 "--values", temp_values_path,
                 "--skip-tests", # Skip tests for cleaner output
                 # Consider adding --kube-version if needed later
            ]
            log_debug(f"Running helm template command: {' '.join(helm_template_cmd)}")

            helm_process = await asyncio.create_subprocess_exec(
                *helm_template_cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
            )
            helm_stdout_bytes, helm_stderr_bytes = await asyncio.wait_for(
                helm_process.communicate(),
                timeout=args.timeout * 2, # Give template more time
            )
            rendered_yaml = helm_stdout_bytes.decode("utf-8", errors="ignore")
            helm_template_stderr = helm_stderr_bytes.decode("utf-8", errors="ignore")
            helm_template_rc = helm_process.returncode

            # Save helm template output
            with open(template_stdout_file, "w", encoding="utf-8") as f: f.write(rendered_yaml)
            with open(template_stderr_file, "w", encoding="utf-8") as f: f.write(helm_template_stderr)

            if helm_template_rc != 0:
                log_debug(f"Helm template command failed with code {helm_template_rc}. Stderr: {helm_template_stderr[:500]}")
                # Continue analysis, but log the error. The comparison will likely show discrepancies.
            else:
                log_debug("Helm template command successful.")

        except asyncio.TimeoutError:
             log_debug(f"Helm template command timed out after {args.timeout * 2} seconds.")
             # Continue analysis, logging the timeout. Comparison won't have template data.
        except Exception as e:
            log_debug(f"Error running helm template: {e}")
            # Continue analysis, logging the error.
        finally:
            # Clean up temp values file
            if temp_values_path and os.path.exists(temp_values_path):
                os.remove(temp_values_path)
                log_debug(f"Removed temporary values file: {temp_values_path}")


        # --- 4. Parse Helm Template Output & Extract Images ---
        log_debug("Parsing helm template output and extracting images...")
        template_images_map: Dict[str, Set[str]] = defaultdict(set) # image -> set(kinds)
        yaml_parsing_errors = []

        if helm_template_rc == 0 and rendered_yaml:
            try:
                documents = yaml.safe_load_all(rendered_yaml)
                doc_index = 0
                for doc in documents:
                    doc_index += 1
                    if not isinstance(doc, dict):
                        # Skip non-dictionary documents (like comments, nulls)
                        continue
                    try:
                        kind = doc.get("kind")
                        # TODO: Implement recursive image extraction from doc based on kind
                        # For now, just log the kind found
                        # if kind:
                        #     log_debug(f"  Processing document {doc_index} with kind: {kind}")
                        # Placeholder:
                        # extracted_images = extract_images_from_doc(doc)
                        # for image in extracted_images:
                        #    template_images_map[image].add(kind or "UnknownKind")

                        extracted_images = extract_images_from_doc(doc)
                        if extracted_images:
                             log_debug(f"  Extracted {len(extracted_images)} images from doc {doc_index} (Kind: {kind or 'Unknown'})")
                             current_kind_str = kind or "UnknownKind"
                             for image in extracted_images:
                                 template_images_map[image].add(current_kind_str)

                    except Exception as e:
                        log_debug(f"Error processing document {doc_index}: {e}")
                        yaml_parsing_errors.append(f"Doc {doc_index}: {e}")
            except yaml.YAMLError as e:
                log_debug(f"Error parsing helm template YAML stream: {e}")
                yaml_parsing_errors.append(f"YAML Stream Error: {e}")
        else:
            log_debug("Skipping template parsing due to helm template failure or empty output.")

        template_images_set = set(template_images_map.keys())
        # log_debug(f"Found {len(template_images_set)} unique images via helm template (placeholder). Actual extraction pending.")
        log_debug(f"Found {len(template_images_set)} unique images via helm template.")

        # --- 5. Compare Image Sets ---
        images_only_in_analyzer = analyzer_images_set - template_images_set
        images_only_in_template = template_images_set - analyzer_images_set
        common_images = analyzer_images_set.intersection(template_images_set)

        log_debug(f"Comparison: Common={len(common_images)}, AnalyzerOnly={len(images_only_in_analyzer)}, TemplateOnly={len(images_only_in_template)}")

        # --- 6. Determine Status & Prepare Results ---
        analysis_status = "UNKNOWN"
        if helm_template_rc != 0:
             analysis_status = "ERROR_TEMPLATE_EXEC"
        elif yaml_parsing_errors:
             analysis_status = "ERROR_TEMPLATE_PARSE"
        elif not images_only_in_analyzer and not images_only_in_template:
             analysis_status = "MATCH"
        elif images_only_in_analyzer and not images_only_in_template:
             analysis_status = "ANALYZER_EXTRA"
        elif not images_only_in_analyzer and images_only_in_template:
             analysis_status = "TEMPLATE_EXTRA"
        else: # Both have unique images
             analysis_status = "MIXED"

        # Prepare result dictionary (structure based on Phase 9.2 plan)
        result_data = {
            "chart_name": chart_name,
            "chart_path": str(chart_path_obj),
            "classification": classification,
            "timestamp": datetime.now().isoformat(),
            "status": analysis_status,
            "analyzer_image_count": len(analyzer_images_set),
            "template_image_count": len(template_images_set),
            "images_common_count": len(common_images),
            "images_only_in_analyzer": sorted(list(images_only_in_analyzer)),
            "images_only_in_template_with_kinds": [
                {"image": img, "kinds": sorted(list(template_images_map[img]))}
                for img in sorted(list(images_only_in_template))
            ], # Placeholder, needs actual kinds -> Now uses actual map
            "analyzer_found_no_images": not analyzer_images_set,
            "template_found_no_images": not template_images_set,
            "helm_template_return_code": helm_template_rc,
            "helm_template_stderr": helm_template_stderr.strip(),
            "yaml_parsing_errors": yaml_parsing_errors,
            "inspect_return_code": inspect_rc,
            "inspect_stderr": inspect_stderr.strip(),
            "discrepancy_warning_found": discrepancy_warning_found,
            "warning_analyzer_count": analyzer_count_from_warning,
            "warning_template_count": template_count_from_warning,
        }


        # --- 7. Append Results to JSON File (using lock) ---
        log_debug("Appending result to JSON file...") # Removed placeholder text
        # TODO: Implement file writing with asyncio.Lock
        async with results_lock:
            try:
                # Read existing data (if file exists)
                existing_data = []
                if subchart_results_file.exists() and subchart_results_file.stat().st_size > 0:
                    with open(subchart_results_file, "r", encoding="utf-8") as f:
                        try:
                            existing_data = json.load(f)
                            if not isinstance(existing_data, list):
                                log_debug(f"Warning: Results file {subchart_results_file} is not a list. Overwriting.")
                                existing_data = []
                        except json.JSONDecodeError:
                            log_debug(f"Warning: Could not decode existing results file {subchart_results_file}. Overwriting.")
                            existing_data = []
                elif subchart_results_file.exists():
                     log_debug(f"Results file {subchart_results_file} exists but is empty. Initializing.")
                     existing_data = []

                # Append new result
                existing_data.append(result_data)

                # Write back to file
                with open(subchart_results_file, "w", encoding="utf-8") as f:
                    json.dump(existing_data, f, indent=2)
                log_debug(f"Successfully appended result for {chart_name} to {subchart_results_file}")

            except Exception as e:
                log_debug(f"Error writing results to {subchart_results_file}: {e}")


        # --- 8. Return Overall Result ---
        # Decide what TestResult status to return based on analysis_status
        final_status = "SUCCESS" # Assume success unless a critical error occurred
        final_category = ""
        final_details = f"Subchart analysis completed. Status: {analysis_status}."

        # Provide more detailed final_details based on findings
        analyzer_count = len(analyzer_images_set)
        template_count = len(template_images_set)
        common_count = len(common_images)
        analyzer_only_count = len(images_only_in_analyzer)
        template_only_count = len(images_only_in_template)

        base_summary = f"Inspect: {analyzer_count}, Template: {template_count}. Common: {common_count}, InspectOnly: {analyzer_only_count}, TemplateOnly: {template_only_count}."

        if analysis_status.startswith("ERROR"):
            final_status = "SUBCHART_ANALYSIS_ERROR"
            final_category = analysis_status # Use the specific error status
            final_details = f"Subchart analysis FAILED. Status: {analysis_status}. {base_summary} Check logs."
        elif not analyzer_images_set and template_images_set:
            final_status = "SUBCHART_DISCREPANCY_INSPECT_MISS" # New status for this key scenario
            final_category = "DISCREPANCY"
            final_details = f"DISCREPANCY: Inspect found 0 images, Template found {template_count} images. {base_summary}"
        elif analyzer_images_set and not template_images_set and helm_template_rc == 0 and not yaml_parsing_errors:
            # This is unusual: inspect found images, but a successful template run found none.
            final_status = "SUBCHART_DISCREPANCY_TEMPLATE_MISS"
            final_category = "DISCREPANCY"
            final_details = f"DISCREPANCY: Inspect found {analyzer_count} images, Template found 0 images (and template was successful). {base_summary}"
        elif not analyzer_images_set and not template_images_set:
            final_details = f"MATCH: No images found by Inspect or Template. {base_summary}"
        elif analysis_status != "MATCH":
            final_details = f"Discrepancy found. Status: {analysis_status}. {base_summary}"
        else: # MATCH and both found images
            final_details = f"MATCH: Image sets are identical. {base_summary}"


        log_debug(f"Subchart analysis finished. Final Status: {final_status}, Details: {final_details[:100]}...")
        return TestResult(
            chart_name=chart_name,
            chart_path=chart_path_obj,
            classification=classification,
            status=final_status,
            category=final_category,
            details=final_details,
            override_duration=0, # Not applicable
            validation_duration=time.monotonic() - start_time,
        )

    except Exception as e:
        log_debug(f"Error during subchart analysis: {e}")
        import traceback
        with open(detailed_log_file, "a", encoding="utf-8") as f:
            traceback.print_exc(file=f)
        # Return failure
        return TestResult(
            chart_name=chart_name,
            chart_path=chart_path_obj, # Use Path object
            classification="UNKNOWN",
            status="SUBCHART_ANALYSIS_ERROR",
            category="UNKNOWN_ERROR", # Or categorize based on exception
            details=f"Unexpected error during subchart analysis: {str(e)[:500]}...",
            override_duration=0,
            validation_duration=time.monotonic() - start_time,
        )


# Repository management functions
def add_helm_repositories():
    """Add Helm repositories."""
    print("Adding Helm repositories...")

    repos = {
        "bitnami": "https://charts.bitnami.com/bitnami",
        "ingress-nginx": "https://kubernetes.github.io/ingress-nginx",
        "prometheus-community": "https://prometheus-community.github.io/helm-charts",
        "grafana": "https://grafana.github.io/helm-charts",
        "fluxcd": "https://fluxcd-community.github.io/helm-charts",
        "apache": "https://apache.github.io/airflow-helm-chart/",
        "aqua": "https://aquasecurity.github.io/helm-charts/",
        "akri": "https://project-akri.github.io/akri/",
        "argo": "https://argoproj.github.io/argo-helm",
        "elastic": "https://helm.elastic.co",
        "jetstack": "https://charts.jetstack.io",
        "hashicorp": "https://helm.releases.hashicorp.com",
        "kong": "https://charts.konghq.com",
        "rancher": "https://releases.rancher.com/server-charts/stable",
        "kedacore": "https://kedacore.github.io/charts",
        "cilium": "https://helm.cilium.io/",
        "istio": "https://istio-release.storage.googleapis.com/charts",
        "kubevela": "https://kubevela.github.io/charts",
    }

    # Add repositories sequentially to avoid race conditions
    for name, url in repos.items():
        try:
            print(f"Adding repository: {name}")
            subprocess.run(
                ["helm", "repo", "add", name, url, "--force-update"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=True,
            )
            # Add a small delay between adds to avoid overwhelming the server
            time.sleep(1)
        except subprocess.CalledProcessError:
            print(f"Warning: Failed to add {name} repository")


def update_helm_repositories():
    """Update Helm repositories sequentially."""
    print("Updating Helm repositories...")
    try:
        # Add a delay before update to prevent rate limits
        time.sleep(2)
        subprocess.run(
            ["helm", "repo", "update"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=True,
        )
        # Add a delay after update to prevent rate limits
        time.sleep(2)
    except subprocess.CalledProcessError:
        print("Warning: Failed to update Helm repositories")


def list_charts() -> List[str]:
    """List available charts from all repositories."""
    result = subprocess.run(
        ["helm", "search", "repo", "-l"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    charts = []
    for line in result.stdout.splitlines()[1:]:  # Skip header line
        parts = line.split()
        if parts:
            charts.append(parts[0])

    return sorted(set(charts))


# Logging functions
def log_error(
    file_path: Path, message: str, error_file: Path = None, stdout_file: Path = None
):
    """Log error details to file and detailed logs directory."""
    with open(file_path, "a") as f:
        f.write(f"{message}\n")
        f.write("Error details:\n")

        # Get error content
        error_content = "No error output captured"
        if (
            error_file
            and os.path.exists(error_file)
            and os.path.getsize(error_file) > 0
        ):
            with open(error_file, "r") as ef:
                error_content = ef.read()

        # Categorize error
        error_category = categorize_error(error_content)
        error_description = ERROR_CATEGORIES.get(error_category, "Unknown error")

        f.write(f"Error Category: {error_category}\n")
        f.write(f"Category Description: {error_description}\n")

        # Log the actual error
        f.write(f"{error_content}\n")

        if (
            stdout_file
            and os.path.exists(stdout_file)
            and os.path.getsize(stdout_file) > 0
        ):
            f.write("Debug output:\n")
            with open(stdout_file, "r") as sf:
                f.write(sf.read())

        f.write("---\n")

    # Save detailed error info to logs directory
    chart_name = os.path.basename(message.split(":")[0])
    error_category_lower = error_category.lower()
    error_log = DETAILED_LOGS_DIR / f"{chart_name}-{error_category_lower}.log"

    with open(error_log, "w") as f:
        f.write("=== Error Details ===\n")
        f.write(f"Chart: {chart_name}\n")
        f.write(f"Category: {error_category}\n")
        f.write(f"Description: {error_description}\n")
        f.write(f"Message: {message}\n")
        f.write("=== Error Content ===\n")
        f.write(f"{error_content}\n")

        if (
            stdout_file
            and os.path.exists(stdout_file)
            and os.path.getsize(stdout_file) > 0
        ):
            f.write("=== Debug Output ===\n")
            with open(stdout_file, "r") as sf:
                f.write(sf.read())


# JSON utilities
def generate_summary_json(total: int, analysis_success: int, override_success: int):
    """Generate summary JSON file."""
    error_counts = {}
    for category in ERROR_CATEGORIES.keys():
        count = 0
        pattern = f"Category: {category}"
        for log_file in DETAILED_LOGS_DIR.glob(f"*-{category.lower()}.log"):
            with open(log_file, "r") as f:
                if pattern in f.read():
                    count += 1
        error_counts[category] = count

    summary = {
        "total_charts": total,
        "analysis_success": analysis_success,
        "override_success": override_success,
        "error_categories": error_counts,
    }

    with open(SUMMARY_JSON_FILE, "w") as f:
        json.dump(summary, f, indent=4)


# Process a single chart
def process_chart(
    chart: str, chart_dir: Path, target_registry: str
) -> Tuple[str, int, int]:
    """Process a chart, returning the chart name, analysis success (0/1), and override success (0/1)."""
    # Removing unused variables
    chart_name = chart

    # Fixing undefined variables by providing explicit parameters
    try:
        # Ensure irr_binary is defined for this context
        irr_binary = BASE_DIR / "bin" / "irr"

        # Create minimal args class with required properties for compatibility
        class Args:
            source_registries = "docker.io"
            target_tag = None
            target_repository = None

        test_args = Args()

        # Test chart override generation
        override_success = 0
        result = test_chart_override(
            (chart, chart_dir), target_registry, irr_binary, None, test_args
        )
        if result and result.status == "SUCCESS":
            override_success = 1

        # Return the results
        return chart_name, 1, override_success
    except Exception as e:
        print(f"Error processing chart {chart_name}: {e}")
        return chart_name, 0, 0


def ensure_default_values():
    """Create default values file with Docker mirror configuration."""
    default_values_dir = Path(__file__).parent / "lib"
    default_values_file = default_values_dir / "default-values.yaml"

    # Create lib directory if it doesn't exist
    default_values_dir.mkdir(exist_ok=True)

    # Write default values file
    with open(default_values_file, "w") as f:
        f.write(DEFAULT_VALUES_CONTENT)

    return default_values_file


def get_chart_classification(chart_path: Path) -> str:
    """
    Analyze chart structure to determine the most appropriate template.
    Returns one of: "BITNAMI", "STANDARD_MAP", "STANDARD_STRING", "DEFAULT"
    """
    try:
        print(
            f"Analyzing chart structure for: {chart_path.name}"
        )  # More concise logging

        chart_yaml_path = chart_path / "Chart.yaml"
        values_yaml_path = chart_path / "values.yaml"

        # --- Check Chart.yaml ---
        chart_data = {}
        if chart_yaml_path.exists():
            print(f"  Checking {chart_yaml_path.name}...")
            with open(chart_yaml_path) as f:
                chart_data = yaml.safe_load(f) or {}  # Ensure dict even if empty/null

                # Check for Bitnami dependency (Strong indicator)
                dependencies = chart_data.get("dependencies", [])
                if isinstance(dependencies, list):
                    for dep in dependencies:
                        if (
                            isinstance(dep, dict)
                            and dep.get("name") == "common"
                            and "bitnami" in dep.get("repository", "")
                        ):
                            print("  → Classified as BITNAMI (common dependency)")
                            return "BITNAMI"
            # Removed scanner check from Chart.yaml - less reliable

        # --- Check values.yaml ---
        values_data = {}
        if values_yaml_path.exists():
            print(f"  Checking {values_yaml_path.name}...")
            with open(values_yaml_path) as f:
                values_data = yaml.safe_load(f) or {}  # Ensure dict even if empty/null

                if not isinstance(values_data, dict):
                    print(
                        f"  Values data is not a dictionary ({type(values_data)}), falling back."
                    )
                    # Fall through to DEFAULT

                # Check for strong Bitnami patterns in values.yaml
                global_data = values_data.get("global", {})
                if isinstance(global_data, dict) and "imageRegistry" in global_data:
                    # Check if volumePermissions.image.repository pattern exists (strong bitnami indicator)
                    volume_permissions = values_data.get("volumePermissions", {})
                    if isinstance(volume_permissions, dict):
                        vp_image = volume_permissions.get("image", {})
                        if isinstance(vp_image, dict) and "repository" in vp_image:
                            print(
                                "  → Classified as BITNAMI (global.imageRegistry + volumePermissions.image.repository)"
                            )
                            return "BITNAMI"
                    # Less strong: global.imageRegistry exists, maybe Bitnami or generic global pattern
                    # Consider adding a dedicated GLOBAL_REGISTRY classification here later if needed.
                    # For now, let's assume it might be Bitnami if global.imageRegistry is present.
                    print(
                        "  → Classified as BITNAMI (heuristic: global.imageRegistry present)"
                    )
                    return "BITNAMI"  # Tentative Bitnami classification

                # Check for explicit image map with repository (STANDARD_MAP)
                image_data = values_data.get(
                    "image", None
                )  # Use None to distinguish missing key
                if isinstance(image_data, dict):
                    if "repository" in image_data:  # Key MUST exist
                        print(
                            "  → Classified as STANDARD_MAP (image map with repository key)"
                        )
                        return "STANDARD_MAP"
                    else:
                        # It's a map, but doesn't contain 'repository'. Could use defaults or globals.
                        # Avoid classifying as STANDARD_MAP. Fall through.
                        print("  Image map found, but 'repository' key is missing.")

                # Check for image as string (STANDARD_STRING)
                elif isinstance(image_data, str):
                    print("  → Classified as STANDARD_STRING (image is a string)")
                    return "STANDARD_STRING"

        # --- Fallback ---
        print("  → Classified as DEFAULT (no specific pattern matched)")
        return "DEFAULT"

    except Exception as e:
        print(
            f"  Warning: Error during chart classification for {chart_path.name}: {e}"
        )
        print("  → Classified as DEFAULT (due to error)")
        return "DEFAULT"


def get_values_content(classification: str, target_registry: str) -> str:
    """
    Get the appropriate values.yaml template based on classification.
    """
    if classification == "BITNAMI":
        template = VALUES_TEMPLATE_BITNAMI
    elif classification == "STANDARD_MAP":
        template = VALUES_TEMPLATE_STANDARD_MAP
    elif classification == "STANDARD_STRING":
        template = VALUES_TEMPLATE_STANDARD_STRING
    else:  # DEFAULT or other unforeseen
        template = VALUES_TEMPLATE_DEFAULT

    # Replace placeholder with actual target registry
    final_template = template.replace(TARGET_REGISTRY_PLACEHOLDER, target_registry)
    # print(f"---\nClassification: {classification}\nGenerated Values:\n{final_template}\n---") # Debugging comment fixed
    return final_template


async def process_charts(
    charts_to_test_names: List[str], args
) -> List[Tuple[str, Path]]:
    """Scan test/chart-cache for .tgz files and return (chart_name, chart_path) tuples."""
    charts_to_process_info: List[Tuple[str, Path]] = []
    for chart_file in CHART_CACHE_DIR.glob("*.tgz"):
        chart_name = chart_file.stem
        if not charts_to_test_names or chart_name in charts_to_test_names:
            charts_to_process_info.append((chart_name, chart_file))
    return charts_to_process_info


async def main():
    """
    Main orchestration function.
    Supports two modes:
    1. Update and Test: Downloads/updates charts and tests them
    2. Local Test: Tests only charts present in test/chart-cache directory
    """
    start_total_time = time.time()
    parser = argparse.ArgumentParser(description="Test Helm chart operations.")
    parser.add_argument("--target-registry", required=True, help="Target registry URL")
    parser.add_argument(
        "--chart-filter", default=None, help="Regex pattern to filter chart names"
    )
    parser.add_argument(
        "--max-charts", type=int, default=None, help="Maximum number of charts to test"
    )
    parser.add_argument(
        "--skip-charts", default=None, help="Comma-separated list of charts to skip"
    )
    parser.add_argument(
        "--max-workers",
        type=int,
        default=os.cpu_count(),
        help="Maximum number of parallel workers",
    )
    parser.add_argument("--no-cache", action="store_true", help="Disable chart caching")
    parser.add_argument(
        "--source-registries",
        required=True,
        help="Comma-separated list of source registries (required)",
    )
    parser.add_argument("--target-tag", default=None, help="Target tag for images")
    parser.add_argument(
        "--target-repository", default=None, help="Target repository for images"
    )
    parser.add_argument(
        "--operation",
        choices=["override", "validate", "both", "override-with-internal-validate", "inspect", "subchart"],
        default="both",
        help="Operation to test: override, validate, both, override-with-internal-validate, inspect, or subchart (analyze discrepancies)",
    )
    parser.add_argument(
        "--debug", action="store_true", help="Enable debug output to console"
    )

    # Add timeout parameter
    parser.add_argument(
        "--timeout",
        type=int,
        default=120,
        help="Timeout in seconds for chart operations",
    )

    # Add lock for subchart analysis results file if needed
    # subchart_results_lock = asyncio.Lock() # Defined within the function now

    args = parser.parse_args()

    # Ensure output directories exist
    TEST_OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    DETAILED_LOGS_DIR.mkdir(parents=True, exist_ok=True)
    CHART_CACHE_DIR.mkdir(parents=True, exist_ok=True)

    irr_binary = BASE_DIR / "bin" / "irr"
    if not irr_binary.exists():
        print(f"Error: irr binary not found at {irr_binary}. Please build it first.")
        sys.exit(1)

    # Get list of charts to test
    charts_to_test_names = []
    for chart_file in CHART_CACHE_DIR.glob("*.tgz"):
        chart_name = chart_file.stem
        charts_to_test_names.append(chart_name)
    print(f"Found {len(charts_to_test_names)} charts in cache directory")

    # Apply skip_charts filter
    skip_list = args.skip_charts.split(",") if args.skip_charts else []
    charts_to_test_names = [c for c in charts_to_test_names if c not in skip_list]

    # Apply chart_filter regex if provided
    if args.chart_filter:
        pattern = re.compile(args.chart_filter)
        charts_to_test_names = [c for c in charts_to_test_names if pattern.search(c)]
        print(f"Filtered to {len(charts_to_test_names)} charts based on pattern: {args.chart_filter}")

    # Apply max_charts limit
    if args.max_charts is not None and len(charts_to_test_names) > args.max_charts:
        charts_to_test_names = charts_to_test_names[: args.max_charts]
        print(f"Limited to testing {len(charts_to_test_names)} charts.")

    if not charts_to_test_names:
        print("No charts selected for testing after filtering.")
        sys.exit(0)

    # --- Process charts ---
    print("\n--- Processing Charts ---")
    charts_to_process_info = await process_charts(charts_to_test_names, args)

    if not charts_to_process_info:
        print("No charts available to test after processing phase.")
        sys.exit(0)

    # --- Test Execution ---
    all_results = []
    executed_operation = None # Track which operation ran

    # Determine which function to run based on the operation
    operation_func = None
    operation_description = ""

    if args.operation == "override":
        operation_func = test_chart_override
        operation_description = "Override Generation (No Internal Validation)"
        executed_operation = "override"
    elif args.operation == "override-with-internal-validate":
        operation_func = test_chart_override_with_internal_validate
        operation_description = "Override Generation (With Internal Validation)"
        executed_operation = "override-with-internal-validate"
    elif args.operation == "inspect":
        operation_func = test_chart_inspect
        operation_description = "Inspect Operation"
        executed_operation = "inspect"
    elif args.operation == "subchart":
        operation_func = test_chart_subchart_analysis
        operation_description = "Subchart Discrepancy Analysis"
        executed_operation = "subchart"
    elif args.operation == "both":
        # "both" implies override first, then validate
        operation_func = test_chart_override # Run override first
        operation_description = "Override Generation (part of 'both')"
        executed_operation = "override"
    elif args.operation == "validate":
        # If only validate is requested, we don't run an initial operation here
        operation_description = "Validation Only"
        executed_operation = "validate"

    # Run the initial operation if needed (override, override-with-internal-validate, inspect, or first part of both)
    if operation_func:
        print(f"\n--- Starting {operation_description} for {len(charts_to_process_info)} charts ---")
        tasks = []
        limiter = asyncio.Semaphore(args.max_workers)

        async def run_task_with_limit(chart_info, op_func):
            async with limiter:
                # Inspect doesn't need target_registry, pass None
                target_reg = args.target_registry if op_func != test_chart_inspect else None
                return await op_func(
                    chart_info, target_reg, irr_binary, None, args
                )

        for chart_name, chart_path in charts_to_process_info:
            task = asyncio.create_task(
                run_task_with_limit((chart_name, chart_path), operation_func)
            )
            tasks.append(task)

        initial_results = await asyncio.gather(*tasks, return_exceptions=True)
        # Store results from the initial operation
        # For 'both', these are the override results
        all_results.extend([r for r in initial_results if isinstance(r, TestResult)])
        if args.operation == "both":
             override_results = initial_results # Keep track for validate step

    # --- Validation Step (Only if 'validate' or 'both' is chosen) ---
    if args.operation in ["validate", "both"]:
        # For validate operation, only test charts that have override files
        charts_to_validate = []
        override_results_dict = {}

        # First, organize override results by chart_name for lookup
        if "override_results" in locals():
            for result in override_results:
                if isinstance(result, TestResult):
                    override_results_dict[result.chart_name] = result

        for chart_name, chart_path in charts_to_process_info:
            override_file_path = TEST_OUTPUT_DIR / f"{chart_name}-values.yaml"
            if override_file_path.exists():
                # Get classification for this chart - default to "UNKNOWN" if not determined
                classification = (
                    get_chart_classification(Path(chart_path)) # Ensure path object
                    if os.path.isdir(chart_path)
                    else "UNKNOWN"
                )

                # Get override duration from previous results if available
                override_duration = 0
                if chart_name in override_results_dict:
                    override_duration = override_results_dict[
                        chart_name
                    ].override_duration

                # Create proper dictionary structure required by test_chart_validate
                chart_info = {
                    "chart_name": chart_name,
                    "chart_path": chart_path,
                    "classification": classification,
                    "override_file_path": override_file_path,
                    "override_duration": override_duration,
                }
                charts_to_validate.append(chart_info)

        if charts_to_validate:
            print(f"\n--- Starting Validation for {len(charts_to_validate)} charts ---")
            tasks = []
            limiter = asyncio.Semaphore(args.max_workers)

            async def run_validate_with_limit(chart_info):
                async with limiter:
                    return await test_chart_validate(
                        chart_info, args.target_registry, irr_binary, None, args
                    )

            for chart_info in charts_to_validate:
                task = asyncio.create_task(run_validate_with_limit(chart_info))
                tasks.append(task)

            validate_results = await asyncio.gather(*tasks, return_exceptions=True)
            all_results.extend(
                [r for r in validate_results if isinstance(r, TestResult)]
            )

    # --- Process Results ---
    print("\n--- Processing Test Results ---")
    processed_results = all_results
    gather_errors = len(all_results) - len(processed_results)

    if gather_errors > 0:
        print(f"Warning: {gather_errors} tasks failed during asyncio.gather.")

    total_tested = len(processed_results)
    success_count = 0
    error_summary = defaultdict(int)
    error_details_rows = []  # For CSV
    error_patterns = defaultdict(int)

    # Update classification stats based on results
    for result in processed_results:
        if result.status == "SUCCESS":
            success_count += 1
            update_classification_stats(result.chart_name, result.classification, True)
        else:
            error_summary[result.status] += 1
            # Use category if available, otherwise status as category
            category = result.category if result.category else result.status
            error_details_rows.append(
                {
                    "Timestamp": datetime.now().isoformat(),
                    "Chart": result.chart_name,
                    "Classification": result.classification,
                    "Status": result.status,
                    "Category": category,
                    "Details": result.details.strip()[:500],  # Limit detail length
                }
            )
            error_patterns[result.details.strip()] += 1
            update_classification_stats(result.chart_name, result.classification, False)

    # --- Determine Output Filenames Based on Operation --- #
    output_prefix = ""
    if executed_operation == "override-with-internal-validate":
        output_prefix = "override_with_internal_validate_"
    elif executed_operation == "validate" or args.operation == "both": # 'validate' or 'both' write to default files
        output_prefix = ""
    elif executed_operation == "override": # Only 'override' writes to default if not part of 'both'
        output_prefix = ""
    # If executed_operation is None (e.g., only validate was run), use default prefix

    error_details_file = TEST_OUTPUT_DIR / f"{output_prefix}error_details.csv"
    error_patterns_file = TEST_OUTPUT_DIR / f"{output_prefix}error_patterns.txt"
    error_summary_file = TEST_OUTPUT_DIR / f"{output_prefix}error_summary.txt"
    # Classification stats always go to the same file
    classification_stats_file = CLASSIFICATION_STATS_FILE

    # Save detailed error CSV
    # error_details_file = TEST_OUTPUT_DIR / "error_details.csv"
    if error_details_rows:
        print(f"  - Writing error details to: {error_details_file}") # Log filename
        with open(error_details_file, "w", newline="", encoding="utf-8") as csvfile:
            fieldnames = [
                "Timestamp",
                "Chart",
                "Classification",
                "Status",
                "Category",
                "Details",
            ]
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(error_details_rows)
    else:
        # Create empty file if no errors, so downstream checks don't fail
        print(f"  - No errors to write to details file: {error_details_file}")
        error_details_file.touch()

    # Save error patterns
    # error_patterns_file = TEST_OUTPUT_DIR / "error_patterns.txt"
    if error_patterns:
        print(f"  - Writing error patterns to: {error_patterns_file}") # Log filename
        with open(error_patterns_file, "w", encoding="utf-8") as f:
            f.write("Error Message Patterns and Counts:\n")
            f.write("=" * 40 + "\n")
            # Sort by count descending
            sorted_patterns = sorted(
                error_patterns.items(), key=lambda item: item[1], reverse=True
            )
            for pattern, count in sorted_patterns:
                f.write(f"Count: {count}\n")
                f.write(f"Pattern:\n{pattern}\n")
                f.write("-" * 40 + "\n")
    else:
        print(f"  - No error patterns to write: {error_patterns_file}")
        error_patterns_file.touch() # Create empty file

    # Save summary counts
    # error_summary_file = TEST_OUTPUT_DIR / "error_summary.txt"
    if error_summary:
        print(f"  - Writing error summary to: {error_summary_file}") # Log filename
        with open(error_summary_file, "w", encoding="utf-8") as f:
            f.write("Error Summary by Status Code:\n")
            f.write("=" * 40 + "\n")
            for status, count in error_summary.items():
                f.write(f"{status}: {count}\n")
    else:
        print(f"  - No error summary to write: {error_summary_file}")
        error_summary_file.touch() # Create empty file

    # Save classification stats
    save_classification_stats()
    print(f"  - Classification stats: {classification_stats_file}")

    # Final summary
    print("\n--- Summary ---")
    print(f"Operation Tested: {args.operation}") # Report which operation ran
    print(f"Total charts targeted: {len(charts_to_process_info)}")
    print(f"Total charts tested: {total_tested}")
    success_percentage = (success_count / total_tested * 100) if total_tested > 0 else 0
    print(
        f"Successful operations: {success_count}/{total_tested} ({success_percentage:.0f}%)"
    )
    print("Error category breakdown:")
    print(f"  - Error details: {error_details_file}")
    print(f"  - Error patterns: {error_patterns_file}")
    print(f"  - Error summary: {error_summary_file}")

    end_total_time = time.time()
    print(f"\nTotal execution time: {end_total_time - start_total_time:.2f} seconds")


if __name__ == "__main__":
    # Check Python version
    if sys.version_info < (3, 8):
        print("Error: This script requires Python 3.8 or higher.")
        sys.exit(1)
    asyncio.run(main())  # Run the async main function
