#!/usr/bin/env python3
"""
Brute force solver for Helm chart validation.
This module implements a solver that finds minimal parameter sets required for successful validation.
"""

import json
import logging
import os
import subprocess
import tarfile
import tempfile
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, List

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

    # Create a temporary values.yaml file
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as tmp:
        # Write params as YAML
        yaml.safe_dump(params, tmp)
        tmp_path = tmp.name

    try:
        # Prepare the command with values file
        validate_cmd = [
            str(irr_binary),
            "validate",
            "--chart-path",
            str(chart_path),
            "--values",
            tmp_path,
        ]

        # Run the command
        result = subprocess.run(
            validate_cmd,
            capture_output=True,
            text=True,
            timeout=60,  # 1-minute timeout
        )

        # Clean up the temporary file
        try:
            os.unlink(tmp_path)
        except OSError:
            pass  # Ignore errors during cleanup

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
        # Clean up the temporary file
        try:
            os.unlink(tmp_path)
        except OSError:
            pass  # Ignore errors during cleanup

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
        # Clean up the temporary file
        try:
            os.unlink(tmp_path)
        except OSError:
            pass  # Ignore errors during cleanup

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
    chart_path,
    parameter_matrix,
    irr_binary,
    max_attempts_per_chart=None,
    target_registry=None,
):
    """Top-level function for processing chart using binary search strategy."""
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    # Extract chart name
    chart_name = chart_path.stem

    # Process parameters based on chart characteristics
    all_parameter_combinations = []

    # Always try with empty parameters first
    all_parameter_combinations.append({})

    # Try with basic kubernetes version
    all_parameter_combinations.append({"kubeVersion": "1.25.0"})

    # Add registry configurations
    if target_registry:
        all_parameter_combinations.append({"image.registry": target_registry})
        all_parameter_combinations.append({"global.imageRegistry": target_registry})

    # Check for schema errors in chart
    # ... (simplified for this implementation)

    # Add special parameters for charts with "provider" in name
    provider = _extract_provider(chart_name)
    if provider:
        if provider == "bitnami":
            all_parameter_combinations.append(
                {
                    "global.imageRegistry": target_registry or "docker.io",
                    "global.storageClass": "standard",
                }
            )

    # Special cases for specific chart types
    if "loki" in chart_name.lower():
        all_parameter_combinations.append(
            {"loki.storage.type": "filesystem", "loki.auth_enabled": "false"}
        )

    if "tempo" in chart_name.lower():
        all_parameter_combinations.append(
            {"tempo.storage.type": "file", "tempo.auth_enabled": "false"}
        )

    # Deduplicate parameter combinations
    unique_combinations = []
    seen = set()

    for params in all_parameter_combinations:
        # Convert to frozenset for hashing
        params_key = frozenset(params.items())
        if params_key not in seen:
            seen.add(params_key)
            unique_combinations.append(params)

    # Try each parameter combination until one works
    errors = []
    for params in unique_combinations:
        result = _test_chart_with_params_task(chart_path, params, irr_binary)
        if result.status == "SUCCESS":
            # Return the successful parameter combination with success indicator
            return {"success": True, "parameters": params, "errors": None}
        else:
            # Keep track of errors
            errors.append(f"{result.category}: {result.details}")

    # If no combination worked, return failure with error information
    return {"success": False, "parameters": None, "errors": errors}


def _process_chart_exhaustive_task(chart_path, parameter_matrix, irr_binary):
    """Top-level function for processing chart using exhaustive strategy.

    Args:
        chart_path: Path to the chart
        parameter_matrix: Dictionary of parameters to test
        irr_binary: Path to the irr binary

    Returns:
        Dictionary of parameters that worked, or None if no parameters worked
    """
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    # Try with empty parameters first - minimal case
    empty_result = _test_chart_with_params_task(chart_path, {}, irr_binary)
    if empty_result.status == "SUCCESS":
        return {"success": True, "parameters": {}, "errors": None}

    # Generate more targeted parameter combinations similar to binary search
    # This is just a starting point; in a true exhaustive approach we would
    # create combinations of all parameters
    parameter_combinations = []

    # Add basic kubeVersion options
    for kube_version in ["1.25.0", "1.28.0"]:
        parameter_combinations.append({"kubeVersion": kube_version})

    # Add registry-related parameters
    parameter_combinations.append({"global.imageRegistry": "docker.io"})
    parameter_combinations.append({"image.registry": "docker.io"})

    # Try persistence options
    parameter_combinations.append({"persistence.enabled": False})
    parameter_combinations.append({"persistence.enabled": True})

    # Add some combined parameters for common scenarios
    parameter_combinations.append(
        {"kubeVersion": "1.28.0", "persistence.enabled": False}
    )

    parameter_combinations.append(
        {"kubeVersion": "1.28.0", "global.imageRegistry": "docker.io"}
    )

    # Try each parameter combination
    errors = []
    for params in parameter_combinations:
        result = _test_chart_with_params_task(chart_path, params, irr_binary)
        if result.status == "SUCCESS":
            return {"success": True, "parameters": params, "errors": None}
        else:
            errors.append(f"{result.category}: {result.details}")

    # If none of the simple combinations worked, try a more complex one
    complex_params = {
        "kubeVersion": "1.28.0",
        "global.imageRegistry": "docker.io",
        "persistence.enabled": False,
        "serviceAccount.create": True,
    }
    result = _test_chart_with_params_task(chart_path, complex_params, irr_binary)
    if result.status == "SUCCESS":
        return {"success": True, "parameters": complex_params, "errors": None}
    else:
        errors.append(f"{result.category}: {result.details}")

    # No successful parameters found
    return {"success": False, "parameters": None, "errors": errors}


def _binary_search_params(chart_path, all_params, irr_binary):
    """Use binary search to find minimal parameter set.

    Args:
        chart_path: Path to the chart
        all_params: List of parameter dictionaries to try
        irr_binary: Path to the irr binary

    Returns:
        Tuple of (success, minimal_params)
    """
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    # Try with no parameters first
    empty_result = _test_chart_with_params_task(chart_path, {}, irr_binary)
    if empty_result.status == "SUCCESS":
        return True, {}

    # Try each parameter set
    for params in all_params:
        result = _test_chart_with_params_task(chart_path, params, irr_binary)
        if result.status == "SUCCESS":
            # Found a working parameter set
            # For now, just return it without minimizing further
            return True, params

    # No successful parameters found
    return False, {}


def _process_chart_task(chart_path, parameter_matrix, irr_binary, strategy):
    """Process a chart with the given parameters.

    Args:
        chart_path: Path to the chart
        parameter_matrix: Dictionary of parameters to test
        irr_binary: Path to the irr binary
        strategy: Strategy to use ('binary' or 'exhaustive')

    Returns:
        Dictionary containing:
        - chart_name: Name of the chart
        - success: Whether any parameter combination worked
        - parameters: Parameter combination that worked (if any)
        - errors: Errors encountered (if any)
    """
    # Ensure chart_path is a Path object
    if not isinstance(chart_path, Path):
        chart_path = Path(chart_path)

    try:
        if strategy == "binary":
            # Use binary search to find minimal parameter set
            result = _process_chart_binary_search_task(
                chart_path, parameter_matrix, irr_binary
            )

            if result:
                return {
                    "chart_name": chart_path.stem,
                    "success": True,
                    "parameters": result,
                    "errors": None,
                }
            else:
                return {
                    "chart_name": chart_path.stem,
                    "success": False,
                    "parameters": None,
                    "errors": "No parameter combination worked",
                }
        elif strategy == "exhaustive":
            # Try all parameter combinations exhaustively
            result = _process_chart_exhaustive_task(
                chart_path, parameter_matrix, irr_binary
            )

            if result:
                return {
                    "chart_name": chart_path.stem,
                    "success": True,
                    "parameters": result,
                    "errors": None,
                }
            else:
                return {
                    "chart_name": chart_path.stem,
                    "success": False,
                    "parameters": None,
                    "errors": "No parameter combination worked",
                }
        else:
            return {
                "chart_name": chart_path.stem,
                "success": False,
                "parameters": None,
                "errors": f"Unknown strategy: {strategy}",
            }
    except Exception as e:
        # Get the traceback
        import traceback

        tb = traceback.format_exc()

        return {
            "chart_name": chart_path.stem,
            "success": False,
            "parameters": None,
            "errors": f"Error: {str(e)}\n{tb}",
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
        solver_output: str = None,
    ):
        """
        Initialize the solver.

        Args:
            max_workers: Number of parallel workers
            output_dir: Directory to store results
            debug: Whether to enable debug output
            checkpoint_interval: How often to save checkpoints (number of charts)
            solver_output: Path to the output file (overrides the default)
        """
        self.max_workers = max_workers
        self.output_dir = output_dir
        self.debug = debug
        self.checkpoint_interval = checkpoint_interval
        self.parameter_matrix = self._build_parameter_matrix()
        self.results = {}

        # Set up output and checkpoint paths
        if solver_output:
            solver_output_path = Path(solver_output)
            # Use the directory of the specified output file
            self.output_dir = solver_output_path.parent
            # Create a checkpoint in the same directory
            self.checkpoint_path = self.output_dir / "solver_checkpoint.json"
            # Set the report path to the specified output file
            self.report_path = solver_output_path
        else:
            self.checkpoint_path = output_dir / "solver_checkpoint.json"
            self.report_path = output_dir / "solver_report.json"

        self.chart_paths = []

        # Set irr_binary to a default value
        self.irr_binary = Path(BASE_DIR) / "bin" / "irr"
        if not self.irr_binary.exists():
            # Look for irr in the current directory
            self.irr_binary = Path("./irr")
            if not self.irr_binary.exists():
                # Try finding it in the PATH
                try:
                    import shutil

                    irr_path = shutil.which("irr")
                    if irr_path:
                        self.irr_binary = Path(irr_path)
                    else:
                        # If we still can't find it, look in a few common locations
                        common_locations = [
                            Path.home() / "bin" / "irr",
                            Path.home() / "go" / "bin" / "irr",
                            Path("/usr/local/bin") / "irr",
                        ]
                        for loc in common_locations:
                            if loc.exists():
                                self.irr_binary = loc
                                break
                except (ImportError, OSError):
                    pass

        print(f"Using irr binary at {self.irr_binary}")
        if not self.irr_binary.exists():
            print("WARNING: irr binary not found! Solver will likely fail.")

        # Create output directory if it doesn't exist
        os.makedirs(self.output_dir, exist_ok=True)

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
        solver_output = getattr(self, "report_path", None)
        solver_output_str = str(solver_output) if solver_output else None

        return (
            self.__class__,
            (
                self.max_workers,
                self.output_dir,
                self.debug,
                self.checkpoint_interval,
                solver_output_str,
            ),
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

    def _group_charts_by_classification(self, chart_paths):
        """
        Group charts by their classification (FAILING, UNREACHABLE, UNKNOWN).
        This is a simplified version for the solver.

        Args:
            chart_paths: List of chart paths to group, can be either tuples (chart_name, chart_path) or Path objects

        Returns:
            Dictionary with classification names as keys and lists of chart paths as values
        """
        classifications = {
            "FAILING": [],
            "UNREACHABLE": [],
            "UNKNOWN": [],  # Default bucket for solver
        }

        # Handle different input formats
        if not chart_paths:
            return classifications

        # Check if we have a list of tuples (chart_name, chart_path)
        if (
            isinstance(chart_paths, list)
            and chart_paths
            and isinstance(chart_paths[0], tuple)
        ):
            # Put each chart in the UNKNOWN bucket
            for chart_name, chart_path in chart_paths:
                classifications["UNKNOWN"].append((chart_name, chart_path))
            return classifications

        # For other formats (single paths or list of paths)
        if not isinstance(chart_paths, list):
            if isinstance(chart_paths, dict):
                # Convert dict of chart_name: chart_path to list
                chart_paths = list(chart_paths.items())
            else:
                # Try to convert to list
                chart_paths = [chart_paths]

        for chart_path in chart_paths:
            # For solver, put all charts in UNKNOWN bucket initially
            classifications["UNKNOWN"].append(chart_path)

        return classifications

    def _process_chart_task(self, chart_path, parameter_matrix, irr_binary, strategy):
        """Process a chart with the given parameters.

        Args:
            chart_path: Path to the chart
            parameter_matrix: Dictionary of parameters to test
            irr_binary: Path to the irr binary
            strategy: Strategy to use ('binary' or 'exhaustive')

        Returns:
            Dictionary containing:
            - chart_name: Name of the chart
            - success: Whether any parameter combination worked
            - parameters: Parameter combination that worked (if any)
            - errors: Errors encountered (if any)
        """
        # Ensure chart_path is a Path object
        if not isinstance(chart_path, Path):
            chart_path = Path(chart_path)

        try:
            if strategy == "binary":
                # Use binary search to find minimal parameter set
                result = _process_chart_binary_search_task(
                    chart_path, parameter_matrix, irr_binary
                )

                if result:
                    return {
                        "chart_name": chart_path.stem,
                        "success": True,
                        "parameters": result,
                        "errors": None,
                    }
                else:
                    return {
                        "chart_name": chart_path.stem,
                        "success": False,
                        "parameters": None,
                        "errors": "No parameter combination worked",
                    }
            elif strategy == "exhaustive":
                # Try all parameter combinations exhaustively
                result = _process_chart_exhaustive_task(
                    chart_path, parameter_matrix, irr_binary
                )

                if result:
                    return {
                        "chart_name": chart_path.stem,
                        "success": True,
                        "parameters": result,
                        "errors": None,
                    }
                else:
                    return {
                        "chart_name": chart_path.stem,
                        "success": False,
                        "parameters": None,
                        "errors": "No parameter combination worked",
                    }
            else:
                return {
                    "chart_name": chart_path.stem,
                    "success": False,
                    "parameters": None,
                    "errors": f"Unknown strategy: {strategy}",
                }
        except Exception as e:
            # Get the traceback
            import traceback

            tb = traceback.format_exc()

            return {
                "chart_name": chart_path.stem,
                "success": False,
                "parameters": None,
                "errors": f"Error: {str(e)}\n{tb}",
            }

    def solve(self, chart_paths):
        """Find the minimal parameter set required for each chart.

        Args:
            chart_paths: List of chart paths to process
        """
        self.chart_paths = chart_paths
        self.logger.info("Starting to solve %d charts.", len(self.chart_paths))

        # Load the checkpoint if it exists
        checkpoint = self._load_checkpoint()
        processed_charts = set(checkpoint.keys())
        self.logger.info(
            "Loaded checkpoint with %d processed charts.", len(processed_charts)
        )

        # Filter out the charts that have already been processed
        filtered_charts = []
        for chart in self.chart_paths:
            chart_name = None
            chart_path = None

            if isinstance(chart, tuple):
                chart_name, chart_path = chart
                if chart_name not in processed_charts:
                    filtered_charts.append(chart)
            else:
                # Assume it's a Path
                chart_name = chart.stem
                chart_path = chart
                if chart_name not in processed_charts:
                    filtered_charts.append(chart)

        self.logger.info(
            "Filtered out %d already processed charts, %d charts remaining.",
            len(processed_charts),
            len(filtered_charts),
        )

        # Process the charts
        for chart in filtered_charts:
            try:
                # Extract chart_name and chart_path if it's a tuple
                if isinstance(chart, tuple):
                    chart_name, chart_path = chart
                else:
                    # Assume it's a Path
                    chart_name = chart.stem
                    chart_path = chart

                self.logger.info("Processing chart %s...", chart_name)

                # Process the chart sequentially
                result = self._process_chart_task(
                    chart_path, self.parameter_matrix, self.irr_binary, "binary"
                )

                # Update the checkpoint
                checkpoint[chart_name] = result
                self._save_checkpoint(checkpoint)

                self.logger.info(
                    "Processed chart %s with result: %s", chart_name, result
                )
            except Exception as e:
                self.logger.error("Error processing chart %s: %s", chart, str(e))

        # Generate a report
        report = self._generate_report(checkpoint)
        return report

    def _load_checkpoint(self) -> Dict:
        print(f"Looking for checkpoint at {self.checkpoint_path}")
        if self.checkpoint_path.exists():
            print("Found checkpoint file, loading it")
            try:
                with open(self.checkpoint_path, "r") as f:
                    checkpoint_data = json.load(f)
                    print(f"Loaded checkpoint with {len(checkpoint_data)} entries")
                    return checkpoint_data
            except Exception as e:
                self.logger.error(f"Error loading checkpoint: {e}")
                print(f"Error loading checkpoint: {e}")
                return {}
        else:
            print("Checkpoint file not found, starting fresh")
            return {}

    def _save_checkpoint(self, checkpoint_data):
        try:
            print(
                f"Saving checkpoint to {self.checkpoint_path} with {len(checkpoint_data)} entries"
            )
            with open(self.checkpoint_path, "w") as f:
                json.dump(checkpoint_data, f)
            print("Checkpoint saved successfully")
        except Exception as e:
            self.logger.error(f"Error saving checkpoint: {e}")
            print(f"Error saving checkpoint: {e}")

    def _generate_report(self, results):
        """Generate a report summarizing the charts and parameters that worked.

        Args:
            results: Dictionary mapping chart names to result dictionaries

        Returns:
            Dictionary with summary statistics and details
        """
        report = {
            "summary": {
                "total_charts": len(results),
                "successful_charts": 0,
                "success_rate": 0.0,
            },
            "classification_stats": defaultdict(
                lambda: {"total": 0, "success": 0, "success_rate": 0.0}
            ),
            "provider_stats": defaultdict(
                lambda: {"total": 0, "success": 0, "success_rate": 0.0}
            ),
            "parameter_stats": {},  # Use regular dict instead of defaultdict
            "error_stats": defaultdict(int),
        }

        # Process results
        for chart_name, result in results.items():
            # Get chart classification
            chart_path = next(
                (
                    p
                    for p in self.chart_paths
                    if isinstance(p, tuple) and p[0] == chart_name
                ),
                None,
            )
            if chart_path is None:
                chart_path = next(
                    (
                        p
                        for p in self.chart_paths
                        if isinstance(p, Path) and p.stem == chart_name
                    ),
                    None,
                )

            if chart_path is None:
                classification = "UNKNOWN"
                provider = None
            else:
                if isinstance(chart_path, tuple):
                    classification = get_chart_classification(chart_path[1])
                    provider = _extract_provider(chart_name)
                else:
                    classification = get_chart_classification(chart_path)
                    provider = _extract_provider(chart_name)

            # Update classification stats
            report["classification_stats"][classification]["total"] += 1

            # Update provider stats
            report["provider_stats"][provider or "null"]["total"] += 1

            # Check if the chart was successful
            if "success" in result and result["success"]:
                # Only count as successful if the parameters were actually successful too
                parameter_success = False

                if "parameters" in result and result["parameters"] is not None:
                    # Handle the new result structure correctly
                    if (
                        isinstance(result["parameters"], dict)
                        and "success" in result["parameters"]
                    ):
                        parameter_success = result["parameters"]["success"]
                    else:
                        # For backwards compatibility with older result format
                        parameter_success = True

                if parameter_success:
                    report["summary"]["successful_charts"] += 1
                    report["classification_stats"][classification]["success"] += 1
                    report["provider_stats"][provider or "null"]["success"] += 1

                    # Count parameter usage for successful charts
                    parameters = result.get("parameters", {})

                    # Navigate nested parameter structure
                    if isinstance(parameters, dict):
                        # If parameters has its own 'parameters' field (nested structure)
                        if (
                            "parameters" in parameters
                            and parameters["parameters"] is not None
                        ):
                            parameters = parameters["parameters"]

                        # Process each parameter individually
                        for param_name, param_value in parameters.items():
                            # Convert param_value to string if it's not a simple type
                            if (
                                not isinstance(param_value, (str, int, float, bool))
                                and param_value is not None
                            ):
                                param_value = str(param_value)

                            # Create the key as "param_name: param_value"
                            param_key = f"{param_name}: {param_value}"

                            # Increment the count for this parameter
                            if param_key not in report["parameter_stats"]:
                                report["parameter_stats"][param_key] = 0
                            report["parameter_stats"][param_key] += 1
                else:
                    # Parameter success was false
                    error_type = "PARAMETER_ERROR"

                    # Try to extract more specific parameter error information
                    if "parameters" in result and isinstance(
                        result["parameters"], dict
                    ):
                        if (
                            "errors" in result["parameters"]
                            and result["parameters"]["errors"]
                        ):
                            error_type = categorize_error(
                                str(result["parameters"]["errors"])
                            )

                    report["error_stats"][error_type] += 1
            else:
                # Chart was not successful at all
                error_type = "UNKNOWN_ERROR"

                # Try to extract a more specific error type from errors
                if "errors" in result and result["errors"]:
                    error_type = categorize_error(str(result["errors"]))

                report["error_stats"][error_type] += 1

        # Calculate success rates
        if report["summary"]["total_charts"] > 0:
            report["summary"]["success_rate"] = (
                report["summary"]["successful_charts"]
                / report["summary"]["total_charts"]
            )

        for classification, stats in report["classification_stats"].items():
            if stats["total"] > 0:
                stats["success_rate"] = stats["success"] / stats["total"]

        for provider, stats in report["provider_stats"].items():
            if stats["total"] > 0:
                stats["success_rate"] = stats["success"] / stats["total"]

        # Convert error stats to percentages
        error_stats_with_percents = {}
        for error_type, count in report["error_stats"].items():
            error_stats_with_percents[error_type] = {
                "count": count,
                "percent": (count / report["summary"]["total_charts"]) * 100
                if report["summary"]["total_charts"] > 0
                else 0,
            }
        report["error_stats"] = error_stats_with_percents

        # Save the report to disk
        with open(self.report_path, "w") as f:
            json.dump(report, f, indent=2)

        return report
