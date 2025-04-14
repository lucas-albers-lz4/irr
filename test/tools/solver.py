#!/usr/bin/env python3
"""
Brute force solver for Helm chart validation.
This module implements a solver that finds minimal parameter sets required for successful validation.
"""

import json
import logging
import os
import shutil
import subprocess
import tarfile
import tempfile
import time
from collections import Counter, defaultdict
from concurrent.futures import ProcessPoolExecutor
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple, Union

import yaml

# Define constants independently to avoid circular imports
MAX_WORKERS_DEFAULT = 4

# Define output directory paths
BASE_DIR = Path(__file__).parent.parent.parent.absolute()
TEST_OUTPUT_DIR = BASE_DIR / "test" / "output"

# Error categories - defined here to avoid circular import
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
    "KUBE_VERSION_ERROR": "Kubernetes version incompatibility",
    "REGISTRY_ERROR": "Registry configuration issue",
    "STORAGE_ERROR": "Persistence or storage configuration issue",
    "AUTH_ERROR": "Authentication or credentials issue",
    "CRD_ERROR": "Custom Resource Definition issue",
    "UNKNOWN_ERROR": "Unclassified error",
}


# Define TestResult class locally to avoid circular imports
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


# Enhanced error categorization function with more specific patterns
def categorize_error(error_msg: str) -> str:
    """
    Categorize error based on error message with enhanced pattern matching.
    Returns the specific error category to help identify required parameters.
    """
    if not error_msg:
        return "UNKNOWN_ERROR"

    error_msg_lower = error_msg.lower()

    # Kubernetes version errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "version constraint",
            "kube version",
            "kubernetes version",
            "not compatible with",
            "incompatible version",
        ]
    ):
        return "KUBE_VERSION_ERROR"

    # Required value errors - expanded patterns
    if any(
        pattern in error_msg_lower
        for pattern in [
            "required value",
            "required field",
            "value is required",
            "cannot be empty",
            "must be set",
            "is required",
            "missing required field",
        ]
    ):
        return "REQUIRED_VALUE_ERROR"

    # YAML parsing errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "failed to parse",
            "yaml:",
            "json:",
            "invalid yaml syntax",
            "unable to parse yaml",
        ]
    ):
        return "YAML_ERROR"

    # Schema validation errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "schema(s)",
            "don't meet the specifications",
            "validation failed",
            "schema validation",
            "specifications of the schema",
            "invalid schema",
        ]
    ):
        return "SCHEMA_ERROR"

    # Type coalescing errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "wrong type for value",
            "coalesce.go",
            "destination for",
            "expected type",
            "cannot merge",
        ]
    ):
        return "COALESCE_ERROR"

    # Registry and image errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "image registry",
            "cannot pull image",
            "registry",
            "repository",
            "docker.io",
            "image pull",
        ]
    ):
        return "REGISTRY_ERROR"

    # Storage and persistence errors
    if any(
        pattern in error_msg_lower
        for pattern in ["persistence", "storage", "volume", "pvc", "storageclass"]
    ):
        return "STORAGE_ERROR"

    # Authentication errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "auth",
            "authentication",
            "credentials",
            "password",
            "secret",
            "access denied",
        ]
    ):
        return "AUTH_ERROR"

    # CRD related errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "crd",
            "custom resource",
            "installcrds",
            "apiversion",
            "resource definition",
        ]
    ):
        return "CRD_ERROR"

    # Template rendering errors
    if any(
        pattern in error_msg_lower
        for pattern in [
            "template",
            "rendering",
            "render",
            "execution error",
            "template error",
            "template failed",
        ]
    ):
        return "TEMPLATE_ERROR"

    # Library chart errors
    if "library charts are not installable" in error_msg_lower:
        return "LIBRARY_ERROR"

    # Deprecated chart errors
    if "chart is deprecated" in error_msg_lower:
        return "DEPRECATED_ERROR"

    # Timeout errors
    if any(
        pattern in error_msg_lower
        for pattern in ["timeout", "timed out", "deadline exceeded"]
    ):
        return "TIMEOUT_ERROR"

    # Configuration errors
    if any(
        pattern in error_msg_lower
        for pattern in ["configuration", "config", "not valid", "invalid configuration"]
    ):
        return "CONFIG_ERROR"

    # If error contains "error" or "failed" but wasn't caught above
    if "error" in error_msg_lower or "failed" in error_msg_lower:
        return "UNKNOWN_ERROR"

    # Default case
    return "UNKNOWN_ERROR"


# Function to get chart classification - simplified version
def get_chart_classification(chart_path: Path) -> str:
    """Determine the classification of a chart based on its path and contents."""
    # Ensure we have a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    # Check if the chart is from Bitnami based on path or Chart.yaml
    chart_yaml_path = chart_path
    if chart_path.is_dir():
        chart_yaml_path = chart_path / "Chart.yaml"
    elif tarfile.is_tarfile(chart_path):
        with tempfile.TemporaryDirectory() as temp_dir:
            with tarfile.open(chart_path) as tar:
                chart_yaml_members = [
                    m for m in tar.getmembers() if m.name.endswith("Chart.yaml")
                ]
                if chart_yaml_members:
                    tar.extract(chart_yaml_members[0], temp_dir)
                    chart_yaml_path = Path(temp_dir) / chart_yaml_members[0].name

    try:
        if chart_yaml_path.exists():
            with open(chart_yaml_path, "r") as f:
                chart_yaml = yaml.safe_load(f)
                if chart_yaml:
                    # Check for Bitnami sources
                    if "sources" in chart_yaml:
                        for source in chart_yaml["sources"]:
                            if "bitnami" in source.lower():
                                return "BITNAMI"
                    # Check for Bitnami maintainers
                    if "maintainers" in chart_yaml:
                        for maintainer in chart_yaml["maintainers"]:
                            if (
                                "name" in maintainer
                                and "bitnami" in maintainer["name"].lower()
                            ):
                                return "BITNAMI"
                            if (
                                "email" in maintainer
                                and "bitnami" in maintainer["email"].lower()
                            ):
                                return "BITNAMI"
    except Exception:
        pass

    # Default to standard if not Bitnami
    return "STANDARD"


# --- Top-level functions for ThreadPoolExecutor ---


def _test_chart_with_params_task(chart_path, params, irr_binary):
    """Top-level function to test a chart with a specific parameter set."""
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    # Prepare the command
    validate_cmd = [str(irr_binary), "validate", "--chart-path", str(chart_path)]

    # Add parameters as --set args
    for param, value in params.items():
        validate_cmd.extend(["--set", f"{param}={value}"])

    # Run the command
    try:
        result = subprocess.run(
            validate_cmd,
            capture_output=True,
            text=True,
            timeout=60,  # 1-minute timeout
        )

        if result.returncode == 0:
            return TestResult(
                chart_name=chart_path.stem,
                chart_path=chart_path,
                classification="UNKNOWN",  # Not relevant for this result
                status="SUCCESS",
                category="SUCCESS",
                details="Validation successful",
                override_duration=0,
                validation_duration=0,
            )
        else:
            error_category = categorize_error(result.stderr)
            return TestResult(
                chart_name=chart_path.stem,
                chart_path=chart_path,
                classification="UNKNOWN",  # Not relevant for this result
                status="VALIDATE_ERROR",
                category=error_category,
                details=f"Error: {result.stderr}",
                override_duration=0,
                validation_duration=0,
            )
    except subprocess.TimeoutExpired:
        return TestResult(
            chart_name=chart_path.stem,
            chart_path=chart_path,
            classification="UNKNOWN",  # Not relevant for this result
            status="TIMEOUT_ERROR",
            category="TIMEOUT_ERROR",
            details="Command timed out after 60 seconds",
            override_duration=0,
            validation_duration=0,
        )
    except Exception as e:
        return TestResult(
            chart_name=chart_path.stem,
            chart_path=chart_path,
            classification="UNKNOWN",  # Not relevant for this result
            status="UNKNOWN_ERROR",
            category="UNKNOWN_ERROR",
            details=f"Error executing command: {e}",
            override_duration=0,
            validation_duration=0,
        )


def _find_minimal_recursive(
    chart_path, candidate_params, current_best_params, irr_binary
):
    """Recursive helper function for binary search minimization."""
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    if not candidate_params:
        return {}  # Should not happen if initial params worked

    if len(candidate_params) == 1:
        # Base case: test the single parameter
        result = _test_chart_with_params_task(chart_path, candidate_params, irr_binary)
        return candidate_params if result.status == "SUCCESS" else current_best_params

    # Try removing the first half
    first_half_names = list(candidate_params.keys())[: len(candidate_params) // 2]
    second_half_names = list(candidate_params.keys())[len(candidate_params) // 2 :]
    second_half_dict = {k: candidate_params[k] for k in second_half_names}

    result_without_first_half = _test_chart_with_params_task(
        chart_path, second_half_dict, irr_binary
    )

    if result_without_first_half.status == "SUCCESS":
        # Second half is sufficient, try minimizing it further
        print(
            f"    Minimized by removing {len(first_half_names)} params, trying further on {len(second_half_dict)}..."
        )
        return _find_minimal_recursive(
            chart_path, second_half_dict, current_best_params, irr_binary
        )
    else:
        # Cannot remove the first half, try removing the second half
        first_half_dict = {k: candidate_params[k] for k in first_half_names}
        result_without_second_half = _test_chart_with_params_task(
            chart_path, first_half_dict, irr_binary
        )

        if result_without_second_half.status == "SUCCESS":
            # First half is sufficient, try minimizing it further
            print(
                f"    Minimized by removing {len(second_half_names)} params, trying further on {len(first_half_dict)}..."
            )
            return _find_minimal_recursive(
                chart_path, first_half_dict, current_best_params, irr_binary
            )
        else:
            # Neither half alone is sufficient. Need parameters from both halves.
            # Combine the minimal sets found by exploring each half.
            print(
                f"    Neither half works alone for {len(candidate_params)} params. Combining results."
            )
            minimal_first = _find_minimal_recursive(
                chart_path, first_half_dict, current_best_params, irr_binary
            )
            minimal_second = _find_minimal_recursive(
                chart_path, second_half_dict, current_best_params, irr_binary
            )
            combined_minimal = {**minimal_first, **minimal_second}

            # Verify the combined set still works (sanity check)
            final_check_result = _test_chart_with_params_task(
                chart_path, combined_minimal, irr_binary
            )
            if final_check_result.status == "SUCCESS":
                print(
                    f"    Combined minimal set verified ({len(combined_minimal)} params)"
                )
                return combined_minimal
            else:
                # Fallback: If combination fails (shouldn't happen logically), return the original candidate set
                print("    Warning: Combined minimal set failed check! Falling back.")
                return candidate_params


def _minimize_parameter_set_task(chart_path, params, irr_binary):
    """Top-level function to find minimal successful parameter set."""
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    if not params:
        return {}

    # If only 1-2 parameters, just return as is
    if len(params) <= 2:
        return params

    print(
        f"  Minimizing parameter set with {len(params)} parameters for {chart_path.name}"
    )

    # Sort params by name to ensure consistent results
    param_names = sorted(params.keys())

    # Test each parameter individually
    for param in param_names:
        test_params = {param: params[param]}
        result = _test_chart_with_params_task(chart_path, test_params, irr_binary)

        if result.status == "SUCCESS":
            # This single parameter is sufficient
            print(f"    Single parameter '{param}' is sufficient for {chart_path.name}")
            return test_params

    # --- Binary search approach ---
    current_best_params = dict(params)  # Start with the full working set

    # Use the top-level helper function for binary search
    minimal_set = _find_minimal_recursive(
        chart_path, dict(params), current_best_params, irr_binary
    )

    print(
        f"    Minimized {chart_path.name} to {len(minimal_set)} parameters from {len(params)}"
    )
    return minimal_set


def _extract_provider(chart_name):
    """Extract provider from chart name."""
    # Common provider patterns
    if "/" in chart_name:
        # Format like bitnami/wordpress
        return chart_name.split("/")[0]

    # Check for known provider patterns in name
    providers = [
        "bitnami",
        "grafana",
        "elastic",
        "prometheus",
        "jetstack",
        "apache",
        "harbor",
        "kong",
        "istio",
        "linkerd",
        "argo",
        "cert-manager",
    ]
    for provider in providers:
        if provider in chart_name.lower():
            return provider

    return None


def _create_all_params(parameter_matrix, target_registry):
    """Create a dictionary with all parameters set to their first non-None value."""
    params = {}

    # Handle parameter matrix directly without expecting 'parameters' key
    for param, values in parameter_matrix.items():
        if not isinstance(values, list):
            continue

        # Replace placeholder in values if necessary
        processed_values = []
        for v in values:
            if isinstance(v, str) and "__TARGET_REGISTRY__" in v:
                processed_values.append(
                    v.replace("__TARGET_REGISTRY__", target_registry)
                )
            else:
                processed_values.append(v)

        for value in processed_values:
            if value is not None:
                params[param] = value
                break
    return params


def _create_parameter_combinations(parameter_matrix, target_registry, max_params=2):
    """Create parameter combinations with bounded complexity."""
    combinations = []

    # Process parameters, replacing placeholders
    processed_params = {}
    for param, values in parameter_matrix.items():
        if not isinstance(values, list):
            continue

        processed_values = []
        for v in values:
            value_to_add = v
            if isinstance(v, str) and "__TARGET_REGISTRY__" in v:
                value_to_add = v.replace("__TARGET_REGISTRY__", target_registry)
            if value_to_add is not None:  # Exclude None values from combinations
                processed_values.append(value_to_add)
        if processed_values:
            processed_params[param] = processed_values

    # Add single parameter combinations
    for param, values in processed_params.items():
        for value in values:
            combinations.append({param: value})

    # Add parameter pairs (limited to reduce explosion)
    if max_params >= 2:
        # Prioritize combinations with kubeVersion
        kube_param = "kubeVersion"
        if kube_param in processed_params:
            kube_values = processed_params[kube_param]
            for kube_value in kube_values:
                for param, values in processed_params.items():
                    if param != kube_param:
                        for value in values:
                            combinations.append({kube_param: kube_value, param: value})

    # Could add triplets and more, but that explodes quickly

    return combinations


def _create_targeted_combinations(
    chart_name, classification, provider, error_categories, target_registry
):
    """Create targeted parameter combinations based on chart properties and errors."""
    combinations = []
    tr = target_registry  # Shorthand

    # Add kube version combinations
    for kube_version in ["1.23.0", "1.25.0", "1.27.0", "1.28.0"]:
        combinations.append({"kubeVersion": kube_version})

    # Add registry combinations
    combinations.append({"global.imageRegistry": tr})
    combinations.append({"image.registry": tr})
    combinations.append({"image": f"{tr}/placeholder:1.0.0"})

    # Add security combinations
    combinations.append({"global.security.allowInsecureImages": True})
    combinations.append({"allowInsecureImages": True})

    # Add combinations for NOTES.txt errors
    if any(
        "NOTES.txt" in error for error in error_categories if isinstance(error, str)
    ):
        combinations.append(
            {
                "kubeVersion": "1.28.0",
                "persistence.enabled": False,
                "serviceAccount.create": True,
            }
        )

    # Add combinations for schema errors
    if "SCHEMA_ERROR" in error_categories:
        combinations.append({"kubeVersion": "1.28.0", "installCRDs": True})

    # Add combined parameters for common providers
    if provider == "bitnami":
        combinations.append(
            {
                "kubeVersion": "1.28.0",
                "global.imageRegistry": tr,
                "global.security.allowInsecureImages": True,
            }
        )

    # Add classification specific combinations
    if classification == "BITNAMI":
        combinations.append(
            {
                "kubeVersion": "1.28.0",
                "global.imageRegistry": tr,
                "global.security.allowInsecureImages": True,
            }
        )
    elif classification == "STANDARD_MAP":
        combinations.append({"kubeVersion": "1.28.0", "image.registry": tr})
    elif classification == "STANDARD_STRING":
        combinations.append(
            {"kubeVersion": "1.28.0", "image": f"{tr}/placeholder:1.0.0"}
        )

    # Add component specific combinations
    components = [
        "loki",
        "tempo",
        "postgresql",
        "redis",
        "mongodb",
        "grafana",
        "prometheus",
    ]
    for component in components:
        if component.lower() in chart_name.lower():
            if component == "loki":
                combinations.append(
                    {
                        "kubeVersion": "1.28.0",
                        "lokiAddress": "http://loki:3100",
                        "loki.storage.type": "filesystem",
                    }
                )
            elif component == "tempo":
                combinations.append(
                    {
                        "kubeVersion": "1.28.0",
                        "tempoAddress.push": "http://tempo:3200",
                        "tempoAddress.query": "http://tempo:3200",
                    }
                )
            elif component in ["postgresql", "redis", "mongodb"]:
                combinations.append(
                    {
                        "kubeVersion": "1.28.0",
                        f"global.{component}.auth.enabled": False,
                        "persistence.enabled": False,
                    }
                )
            # Add more component-specific combinations

    # Deduplicate
    unique_combinations = []
    seen_combinations = set()
    for combo in combinations:
        combo_tuple = tuple(sorted(combo.items()))
        if combo_tuple not in seen_combinations:
            unique_combinations.append(combo)
            seen_combinations.add(combo_tuple)

    return unique_combinations


def _process_chart_binary_search_task(
    chart_info, irr_binary, parameter_matrix, max_attempts_per_chart, target_registry
):
    """Top-level function for processing chart using binary search strategy. Includes extraction."""
    chart_name, tgz_path = chart_info
    # Ensure tgz_path is a Path object
    if not isinstance(tgz_path, Path):
        tgz_path = Path(tgz_path)

    temp_dir = None
    extracted_chart_path = None

    try:
        # 1. Create temp directory and extract chart
        temp_dir = Path(tempfile.mkdtemp(prefix=f"solver-{chart_name}-"))
        try:
            with tarfile.open(tgz_path, "r:gz") as tar:
                # Find the base directory within the tarball
                chart_base_dir = next(
                    (
                        m.name
                        for m in tar.getmembers()
                        if m.isdir() and "/" not in m.name.strip("/")
                    ),
                    None,
                )
                if not chart_base_dir:
                    # Handle cases where tarball might not have a single top-level dir
                    # Or extract directly if no obvious base dir
                    tar.extractall(path=temp_dir)
                    # In this case, assume the temp_dir itself is the chart path
                    extracted_chart_path = temp_dir
                    # We might need to refine this logic if charts have weird structures
                    # Let's try finding a Chart.yaml to be more robust
                    potential_chart_dirs = list(temp_dir.glob("*/Chart.yaml"))
                    if len(potential_chart_dirs) == 1:
                        extracted_chart_path = potential_chart_dirs[0].parent
                    elif len(potential_chart_dirs) > 1:
                        print(
                            f"Warning: Found multiple Chart.yaml files in {temp_dir} for {chart_name}. Using temp dir root."
                        )
                        extracted_chart_path = temp_dir  # Fallback
                    else:
                        print(
                            f"Warning: No Chart.yaml found directly in {temp_dir} for {chart_name}. Using temp dir root."
                        )
                        extracted_chart_path = temp_dir  # Fallback

                else:
                    tar.extractall(path=temp_dir)
                    extracted_chart_path = temp_dir / chart_base_dir

            if not extracted_chart_path or not extracted_chart_path.is_dir():
                raise FileNotFoundError(
                    f"Could not find extracted chart directory in {temp_dir} for {chart_name}"
                )

        except tarfile.ReadError as e:
            print(f"Error extracting tar file {tgz_path}: {e}")
            # Return a result indicating extraction failure
            return chart_name, {
                "classification": "EXTRACTION_ERROR",
                "provider": _extract_provider(chart_name),
                "attempts": [],
                "minimal_success_params": None,
                "error_categories": ["EXTRACTION_ERROR"],
            }
        except Exception as e:
            print(f"Unexpected error during extraction for {chart_name}: {e}")
            import traceback

            traceback.print_exc()
            return chart_name, {
                "classification": "EXTRACTION_ERROR",
                "provider": _extract_provider(chart_name),
                "attempts": [],
                "minimal_success_params": None,
                "error_categories": ["EXTRACTION_ERROR"],
            }

        # 2. Get classification and provider (now that we have the extracted path)
        classification = get_chart_classification(extracted_chart_path)
        provider = _extract_provider(chart_name)

        chart_results = {
            "classification": classification,
            "provider": provider,
            "attempts": [],
            "minimal_success_params": None,
            "error_categories": [],
        }

        print(
            f"Processing chart: {chart_name} (Extracted to: {extracted_chart_path}, Classification: {classification}, Provider: {provider})"
        )

        # 3. Run the actual test logic using extracted_chart_path
        # Try with no parameters first (baseline)
        result = _test_chart_with_params_task(extracted_chart_path, {}, irr_binary)
        chart_results["attempts"].append(
            {
                "parameters": {},
                "success": result.status == "SUCCESS",
                "error_category": result.category
                if result.status != "SUCCESS"
                else None,
            }
        )

        if result.status == "SUCCESS":
            chart_results["minimal_success_params"] = {}
            print(f"  Success with no parameters for {chart_name}")
            return chart_name, chart_results

        # Track error category
        if result.category and result.category not in chart_results["error_categories"]:
            chart_results["error_categories"].append(result.category)

        # Try only kubeVersion=1.28.0
        kube_params = {"kubeVersion": "1.28.0"}
        result = _test_chart_with_params_task(
            extracted_chart_path, kube_params, irr_binary
        )
        chart_results["attempts"].append(
            {
                "parameters": kube_params,
                "success": result.status == "SUCCESS",
                "error_category": result.category
                if result.status != "SUCCESS"
                else None,
            }
        )

        if result.status == "SUCCESS":
            chart_results["minimal_success_params"] = kube_params
            print(f"  Success with kubeVersion only for {chart_name}")
            return chart_name, chart_results

        # Try with ALL parameters
        all_params = _create_all_params(parameter_matrix, target_registry)
        result = _test_chart_with_params_task(
            extracted_chart_path, all_params, irr_binary
        )
        chart_results["attempts"].append(
            {
                "parameters": all_params,
                "success": result.status == "SUCCESS",
                "error_category": result.category
                if result.status != "SUCCESS"
                else None,
            }
        )

        if result.status == "SUCCESS":
            print(f"  Success with all parameters for {chart_name}, minimizing...")
            chart_results["minimal_success_params"] = _minimize_parameter_set_task(
                extracted_chart_path, all_params, irr_binary
            )
            return chart_name, chart_results

        # Try targeted combinations
        param_sets = _create_targeted_combinations(
            chart_name,
            classification,
            provider,
            chart_results["error_categories"],
            target_registry,
        )

        for params in param_sets:
            if len(chart_results["attempts"]) >= max_attempts_per_chart:
                print(
                    f"  Reached maximum attempts ({max_attempts_per_chart}) for {chart_name}"
                )
                break

            result = _test_chart_with_params_task(
                extracted_chart_path, params, irr_binary
            )
            chart_results["attempts"].append(
                {
                    "parameters": params,
                    "success": result.status == "SUCCESS",
                    "error_category": result.category
                    if result.status != "SUCCESS"
                    else None,
                }
            )

            if result.status == "SUCCESS":
                print(f"  Success with parameter set for {chart_name}, minimizing...")
                chart_results["minimal_success_params"] = _minimize_parameter_set_task(
                    extracted_chart_path, params, irr_binary
                )
                return chart_name, chart_results

            if (
                result.category
                and result.category not in chart_results["error_categories"]
            ):
                chart_results["error_categories"].append(result.category)

        print(
            f"  No successful parameter set found for {chart_name} after {len(chart_results['attempts'])} attempts"
        )
        return chart_name, chart_results

    finally:
        # 4. Clean up temp directory
        if temp_dir and temp_dir.exists():
            shutil.rmtree(temp_dir)


def _process_chart_exhaustive_task(
    chart_info, irr_binary, parameter_matrix, max_attempts_per_chart, target_registry
):
    """Top-level function for processing chart using exhaustive brute force strategy. Includes extraction."""
    chart_name, tgz_path = chart_info
    # Ensure tgz_path is a Path object
    if not isinstance(tgz_path, Path):
        tgz_path = Path(tgz_path)

    temp_dir = None
    extracted_chart_path = None

    try:
        # 1. Create temp directory and extract chart
        temp_dir = Path(tempfile.mkdtemp(prefix=f"solver-{chart_name}-"))
        try:
            with tarfile.open(tgz_path, "r:gz") as tar:
                # Similar extraction logic as binary search task
                chart_base_dir = next(
                    (
                        m.name
                        for m in tar.getmembers()
                        if m.isdir() and "/" not in m.name.strip("/")
                    ),
                    None,
                )
                if not chart_base_dir:
                    tar.extractall(path=temp_dir)
                    potential_chart_dirs = list(temp_dir.glob("*/Chart.yaml"))
                    if len(potential_chart_dirs) == 1:
                        extracted_chart_path = potential_chart_dirs[0].parent
                    else:  # Fallback if structure is ambiguous
                        extracted_chart_path = temp_dir
                else:
                    tar.extractall(path=temp_dir)
                    extracted_chart_path = temp_dir / chart_base_dir

            if not extracted_chart_path or not extracted_chart_path.is_dir():
                raise FileNotFoundError(
                    f"Could not find extracted chart directory in {temp_dir} for {chart_name}"
                )
        except tarfile.ReadError as e:
            print(f"Error extracting tar file {tgz_path}: {e}")
            return chart_name, {
                "classification": "EXTRACTION_ERROR",
                "provider": _extract_provider(chart_name),
                "attempts": [],
                "minimal_success_params": None,
                "error_categories": ["EXTRACTION_ERROR"],
            }
        except Exception as e:
            print(f"Unexpected error during extraction for {chart_name}: {e}")
            import traceback

            traceback.print_exc()
            return chart_name, {
                "classification": "EXTRACTION_ERROR",
                "provider": _extract_provider(chart_name),
                "attempts": [],
                "minimal_success_params": None,
                "error_categories": ["EXTRACTION_ERROR"],
            }

        # 2. Get classification and provider
        classification = get_chart_classification(extracted_chart_path)
        provider = _extract_provider(chart_name)

        chart_results = {
            "classification": classification,
            "provider": provider,
            "attempts": [],
            "minimal_success_params": None,
            "error_categories": [],
        }

        print(
            f"Processing chart: {chart_name} (Extracted to: {extracted_chart_path}, Classification: {classification}, Provider: {provider})"
        )

        # 3. Run exhaustive test logic
        # Try with no parameters first
        result = _test_chart_with_params_task(extracted_chart_path, {}, irr_binary)
        chart_results["attempts"].append(
            {
                "parameters": {},
                "success": result.status == "SUCCESS",
                "error_category": result.category
                if result.status != "SUCCESS"
                else None,
            }
        )

        if result.status == "SUCCESS":
            chart_results["minimal_success_params"] = {}
            print(f"  Success with no parameters for {chart_name}")
            return chart_name, chart_results

        if result.category and result.category not in chart_results["error_categories"]:
            chart_results["error_categories"].append(result.category)

        # Generate combinations
        combinations = _create_parameter_combinations(parameter_matrix, target_registry)
        print(f"  Testing {len(combinations)} parameter combinations for {chart_name}")
        combinations.sort(key=lambda x: len(x))
        combinations = combinations[: max_attempts_per_chart - 1]

        for params in combinations:
            result = _test_chart_with_params_task(
                extracted_chart_path, params, irr_binary
            )
            chart_results["attempts"].append(
                {
                    "parameters": params,
                    "success": result.status == "SUCCESS",
                    "error_category": result.category
                    if result.status != "SUCCESS"
                    else None,
                }
            )

            if result.status == "SUCCESS":
                print(f"  Success with parameter set for {chart_name}, minimizing...")
                chart_results["minimal_success_params"] = _minimize_parameter_set_task(
                    extracted_chart_path, params, irr_binary
                )
                return chart_name, chart_results

            if (
                result.category
                and result.category not in chart_results["error_categories"]
            ):
                chart_results["error_categories"].append(result.category)

        print(
            f"  No successful parameter set found for {chart_name} after {len(chart_results['attempts'])} attempts"
        )
        return chart_name, chart_results

    finally:
        # 4. Clean up temp directory
        if temp_dir and temp_dir.exists():
            shutil.rmtree(temp_dir)


def _process_chart_task(chart_info, solver_config):
    """Top-level function for processing chart based on selected strategy. Dispatches to appropriate strategy."""
    chart_name, chart_path = chart_info
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    try:
        # Get strategy execution function
        if solver_config["strategy"] == "binary":
            return _process_chart_binary_search_task(
                chart_info,
                solver_config["irr_binary"],
                solver_config["parameter_matrix"],
                solver_config["max_attempts_per_chart"],
                solver_config["target_registry"],
            )
        elif solver_config["strategy"] == "exhaustive":
            return _process_chart_exhaustive_task(
                chart_info,
                solver_config["irr_binary"],
                solver_config["parameter_matrix"],
                solver_config["max_attempts_per_chart"],
                solver_config["target_registry"],
            )
        else:  # Default to binary search
            print(
                f"Warning: Unknown solver strategy '{solver_config['strategy']}', defaulting to binary search."
            )
            return _process_chart_binary_search_task(
                chart_info,
                solver_config["irr_binary"],
                solver_config["parameter_matrix"],
                solver_config["max_attempts_per_chart"],
                solver_config["target_registry"],
            )
    except Exception as e:
        # Handle any exceptions that might occur during processing
        import traceback

        traceback.print_exc()
        return chart_name, {
            "classification": "PROCESSING_ERROR",
            "provider": _extract_provider(chart_name),
            "attempts": [],
            "minimal_success_params": None,
            "error_categories": ["PROCESSING_ERROR"],
            "error_details": str(e),
        }


# --- ChartSolver Class ---


class ChartSolver:
    """
    Brute force solver for Helm chart validation.
    Finds the minimal parameter set required for successful validation.
    """

    def __init__(
        self,
        max_workers: int = MAX_WORKERS_DEFAULT,
        output_dir: Path = TEST_OUTPUT_DIR,
        debug: bool = False,
        checkpoint_interval: int = 50,
    ):
        """
        Initialize the solver.

        Args:
            max_workers: Number of parallel workers
            output_dir: Directory to store results
            debug: Whether to enable debug output
            checkpoint_interval: How often to save checkpoints (number of charts)
        """
        self.max_workers = max_workers
        self.output_dir = output_dir
        self.debug = debug
        self.checkpoint_interval = checkpoint_interval
        self.parameter_matrix = self._build_parameter_matrix()
        self.results = {}
        self.checkpoint_path = output_dir / "solver_checkpoint.json"

        # Create output directory if it doesn't exist
        os.makedirs(output_dir, exist_ok=True)

        # Setup logging
        logging.basicConfig(
            level=logging.DEBUG if debug else logging.INFO,
            format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
            handlers=[
                logging.FileHandler(output_dir / "solver.log"),
                logging.StreamHandler(),
            ],
        )
        self.logger = logging.getLogger("ChartSolver")

    def __reduce__(self):
        """
        Make ChartSolver picklable for multiprocessing.
        Returns a tuple of (callable, args) that allows the instance to be recreated.
        """
        # Return a tuple with the class and the arguments needed to recreate this instance
        return (
            self.__class__,
            (self.max_workers, self.output_dir, self.debug, self.checkpoint_interval),
        )

    def _build_parameter_matrix(self) -> Dict[str, List[Any]]:
        """
        Build a matrix of parameters to test.
        Returns a dictionary mapping parameter names to lists of possible values.
        """
        # Start with an empty matrix - true data-driven approach
        matrix = {}

        # Kubernetes versions to test - most commonly needed parameter
        matrix["kubeVersion"] = [
            None,  # No version specified
            "1.13.0",
            "1.16.0",
            "1.19.0",
            "1.22.0",
            "1.25.0",
            "1.28.0",
        ]

        # Registry configuration options
        matrix["registry"] = [
            None,  # No registry config
            {"insecure": True},  # Insecure registry
            {"insecure": False},  # Secure registry
        ]

        # Storage/Persistence options
        matrix["persistence"] = [
            None,  # No persistence config
            {"enabled": False},  # Disable persistence
            {"enabled": True},  # Enable persistence
        ]

        # Authentication options
        matrix["auth"] = [
            None,  # No auth config
            {"enabled": False},  # Auth disabled
            {"enabled": True},  # Auth enabled
        ]

        # Global settings that are commonly required
        matrix["global"] = [
            None,  # No global settings
            {},  # Empty global object
            {"imageRegistry": "docker.io"},  # Common global settings
        ]

        # Common base values
        matrix["base_values"] = [
            None,  # No base values
            {},  # Empty object
            {"fullnameOverride": "release-name"},  # Override release name
        ]

        return matrix

    def solve(
        self,
        chart_paths: List[Union[Path, Tuple[str, Path]]],
        output_file: Optional[str] = None,
    ) -> Dict:
        """
        Find the minimal parameter set required for each chart.

        Args:
            chart_paths: List of chart paths or tuples of (chart_name, chart_path)
            output_file: Optional path to save results

        Returns:
            Dictionary with results for each chart
        """
        # Check for existing checkpoint
        loaded_checkpoint = self._load_checkpoint()
        if loaded_checkpoint:
            self.results = loaded_checkpoint
            # Filter out charts that have already been processed
            chart_paths = [
                p
                for p in chart_paths
                if (
                    str(p) not in self.results
                    if not isinstance(p, tuple)
                    else str(p[1]) not in self.results
                )
            ]
            self.logger.info(
                f"Loaded checkpoint with {len(self.results)} charts. {len(chart_paths)} charts remaining."
            )

        if not chart_paths:
            self.logger.info("No charts to process!")
            return self.results

        # Process chart paths and convert to (name, path) tuples if needed
        processed_chart_paths = []
        for chart_path in chart_paths:
            if isinstance(chart_path, tuple):
                chart_name, path = chart_path
                # Ensure path is a Path object
                if not isinstance(path, Path):
                    path = Path(path)
                processed_chart_paths.append((chart_name, path))
            else:
                # Ensure chart_path is a Path object
                if not isinstance(chart_path, Path):
                    chart_path = Path(chart_path)
                chart_name = chart_path.stem
                processed_chart_paths.append((chart_name, chart_path))

        # Now use processed_chart_paths for actual processing
        # Get the absolute path to the irr binary
        irr_binary = Path(os.path.join(BASE_DIR, "bin", "irr")).absolute()
        if not irr_binary.exists():
            self.logger.error(f"irr binary not found at {irr_binary}")
            return self.results

        # Process charts using top-level functions
        start_time = time.time()
        processed_count = 0

        # Get parameter matrix once to avoid pickling issues
        parameter_matrix = self._build_parameter_matrix()
        target_registry = "docker.io"  # Default target registry
        max_attempts = 10  # Default max attempts per chart

        # Process charts with ProcessPoolExecutor
        with ProcessPoolExecutor(max_workers=self.max_workers) as executor:
            # Submit jobs
            future_to_chart = {}
            for chart_path in processed_chart_paths:
                # Use a top-level function instead of a class method
                future = executor.submit(
                    _process_chart_task,
                    chart_path,
                    {
                        "strategy": "binary",  # Default strategy
                        "irr_binary": irr_binary,
                        "parameter_matrix": parameter_matrix,
                        "max_attempts_per_chart": max_attempts,
                        "target_registry": target_registry,
                    },
                )
                future_to_chart[future] = chart_path

            # Process results as they complete
            for future in future_to_chart:
                chart_path = future_to_chart[future]
                chart_key = (
                    str(chart_path[1])
                    if isinstance(chart_path, tuple)
                    else str(chart_path)
                )

                try:
                    chart_name, chart_results = future.result()
                    self.results[chart_key] = chart_results
                    processed_count += 1

                    # Log progress
                    if processed_count % 10 == 0:
                        elapsed = time.time() - start_time
                        charts_per_second = (
                            processed_count / elapsed if elapsed > 0 else 0
                        )
                        self.logger.info(
                            f"Processed {processed_count}/{len(processed_chart_paths)} charts. "
                            f"({charts_per_second:.2f} charts/s)"
                        )

                    # Save checkpoint periodically
                    if processed_count % self.checkpoint_interval == 0:
                        self._save_checkpoint()
                except Exception as e:
                    self.logger.error(f"Error processing chart {chart_path}: {str(e)}")
                    self.results[chart_key] = {
                        "status": "ERROR",
                        "error": str(e),
                    }

        # Save final results
        self._save_checkpoint()

        # Generate report
        if output_file:
            self._generate_report(output_file)

        total_time = time.time() - start_time
        self.logger.info(
            f"Completed processing {processed_count} charts in {total_time:.2f}s"
        )
        return self.results

    def _load_checkpoint(self) -> Dict:
        """
        Load results from a checkpoint file if it exists.

        Returns:
            Dictionary with results or empty dictionary if no checkpoint
        """
        if self.checkpoint_path.exists():
            try:
                with open(self.checkpoint_path, "r") as f:
                    return json.load(f)
            except Exception as e:
                self.logger.error(f"Error loading checkpoint: {str(e)}")
        return {}

    def _save_checkpoint(self) -> None:
        """Save current results to a checkpoint file."""
        try:
            with open(self.checkpoint_path, "w") as f:
                json.dump(self.results, f, indent=2)
            self.logger.info(f"Checkpoint saved with {len(self.results)} charts")
        except Exception as e:
            self.logger.error(f"Error saving checkpoint: {str(e)}")

    def _generate_report(self, output_file: str) -> None:
        """
        Generate a report of the results.

        Args:
            output_file: Path to save the report
        """
        report = {
            "summary": {
                "total_charts": len(self.results),
                "successful_charts": sum(
                    1
                    for r in self.results.values()
                    if "minimal_params" in r and r["minimal_params"] is not None
                ),
                "failed_charts": sum(
                    1
                    for r in self.results.values()
                    if "minimal_params" not in r or r["minimal_params"] is None
                ),
                "error_categories": Counter(),
            },
            "charts": self.results,
            "parameter_statistics": self._generate_parameter_statistics(),
        }

        # Collect error categories
        for chart_results in self.results.values():
            if "error_categories" in chart_results:
                for category, count in chart_results["error_categories"].items():
                    report["summary"]["error_categories"][category] = (
                        report["summary"]["error_categories"].get(category, 0) + count
                    )

        # Save report
        with open(output_file, "w") as f:
            json.dump(report, f, indent=2)
        self.logger.info(f"Report saved to {output_file}")

    def _generate_parameter_statistics(self) -> Dict:
        """
        Generate statistics about which parameters were most commonly required.

        Returns:
            Dictionary with parameter statistics
        """
        stats = {"required_parameters": Counter(), "parameter_combinations": Counter()}

        # Collect required parameters
        for chart_results in self.results.values():
            if (
                "minimal_params" in chart_results
                and chart_results["minimal_params"] is not None
            ):
                # Count individual parameters
                for param in chart_results["minimal_params"].keys():
                    stats["required_parameters"][param] += 1

                # Count parameter combinations (as frozensets)
                param_set = frozenset(chart_results["minimal_params"].keys())
                stats["parameter_combinations"][str(param_set)] += 1

        return stats


# Utility functions for analyzing solver results
def load_solver_results(results_file):
    """Load solver results from a JSON file."""
    with open(results_file, "r") as f:
        data = json.load(f)

    # Check if results follow the new structure with a 'charts' key
    if "charts" in data:
        return data["charts"]

    # Otherwise assume it's the old format
    return data


def get_success_rate(results):
    """Calculate success rate from solver results."""
    total = len(results)
    successful = sum(
        1
        for r in results.values()
        if "minimal_params" in r and r["minimal_params"] is not None
    )
    return (successful / total) * 100 if total > 0 else 0


def get_parameter_distribution(results):
    """Analyze parameter distribution across successful charts."""
    param_counts = defaultdict(int)
    param_values = defaultdict(lambda: defaultdict(int))

    for result in results.values():
        if "minimal_params" in result and result["minimal_params"] is not None:
            for param_name, param_value in result["minimal_params"].items():
                param_counts[param_name] += 1
                param_values[param_name][str(param_value)] += 1

    return {
        "counts": dict(param_counts),
        "values": {k: dict(v) for k, v in param_values.items()},
    }


def group_charts_by_minimal_params(results):
    """Group charts by their minimal parameter sets."""
    groups = defaultdict(list)

    for chart_path, result in results.items():
        if "minimal_params" in result and result["minimal_params"] is not None:
            # Convert the minimal params to a hashable form (tuple of sorted items)
            param_tuple = tuple(
                sorted([(k, str(v)) for k, v in result["minimal_params"].items()])
            )
            groups[param_tuple].append(chart_path)

    # Convert to a more readable format
    readable_groups = {}
    for param_tuple, charts in groups.items():
        param_dict = {k: v for k, v in param_tuple}
        key = json.dumps(param_dict, sort_keys=True)
        readable_groups[key] = charts

    return readable_groups


def compare_results(results1, results2):
    """Compare two sets of solver results and identify differences."""
    charts1 = set(results1.keys())
    charts2 = set(results2.keys())

    only_in_1 = charts1 - charts2
    only_in_2 = charts2 - charts1
    in_both = charts1.intersection(charts2)

    # For charts in both, compare the minimal parameter sets
    param_differences = {}
    for chart in in_both:
        params1 = results1[chart].get("minimal_params")
        params2 = results2[chart].get("minimal_params")

        if params1 != params2:
            param_differences[chart] = {"results1": params1, "results2": params2}

    return {
        "only_in_first": list(only_in_1),
        "only_in_second": list(only_in_2),
        "param_differences": param_differences,
        "success_rate1": get_success_rate(results1),
        "success_rate2": get_success_rate(results2),
    }


def generate_report(results, output_file):
    """Generate a comprehensive analysis report from solver results."""
    # Calculate success rate
    success_rate = get_success_rate(results)

    # Get parameter distribution
    param_dist = get_parameter_distribution(results)

    # Group charts by minimal params
    groups = group_charts_by_minimal_params(results)

    # Compile report
    report = {
        "success_rate": success_rate,
        "total_charts": len(results),
        "successful_charts": sum(
            1
            for r in results.values()
            if "minimal_params" in r and r["minimal_params"] is not None
        ),
        "parameter_distribution": param_dist,
        "chart_groups": groups,
    }

    # Write report to file
    with open(output_file, "w") as f:
        json.dump(report, f, indent=2)

    return report


if __name__ == "__main__":
    # Example usage:
    # results = load_solver_results("solver_results.json")
    # report = generate_report(results, "solver_analysis.json")
    # print(f"Success rate: {report['success_rate']:.2f}%")
    pass
