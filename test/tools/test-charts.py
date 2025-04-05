#!/usr/bin/env python3
"""
Test script for helm-image-override.

This script tests the override generation and template validation for Helm charts.
It assumes charts have already been downloaded to the cache directory (e.g., by pull-charts.py).
It does NOT pull charts from repositories.
"""

import argparse
import concurrent.futures
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import tarfile
import yaml
import time
from pathlib import Path
from typing import Dict, List, Tuple, Any, Optional
from collections import defaultdict
import csv
from datetime import datetime
import asyncio
from dataclasses import dataclass

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
    "OVERRIDE_ERROR": "helm-image-override command failed",
    "TEMPLATE_ERROR": "Helm template rendering failed",
    "SCHEMA_ERROR": "Chart schema validation error",
    "YAML_ERROR": "YAML parsing or syntax error",
    "REQUIRED_VALUE_ERROR": "Required chart value is missing",
    "COALESCE_ERROR": "Type mismatch in value coalescing",
    "LIBRARY_ERROR": "Library chart installation attempted",
    "DEPRECATED_ERROR": "Chart is deprecated",
    "CONFIG_ERROR": "Chart configuration error",
    "TIMEOUT_ERROR": "Processing timed out",
    "UNKNOWN_ERROR": "Unclassified error"
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
VALUES_TEMPLATE_STANDARD_MAP = f"""
global: {{}} # Assume global might exist but add no specific keys

# Service structure required by many common libraries
# Use nested map structure for ports
service:
  main: # Common primary service name
    ports:
      http: # Common port name
        enabled: true
        port: 80

# Assume image map exists, provide SemVer tag
image:
  repository: placeholderRepo # Will be overridden
  tag: "1.0.0" # Use valid SemVer default
  pullPolicy: IfNotPresent

# Remove serviceAccount.create: true as it causes schema errors
# serviceAccount:
#  create: true # Often needed

# Required by specific charts, avoid adding globally if possible
# clusterName: "test-cluster"
# azureTenantID: "dummy-tenant-id"
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
VALUES_TEMPLATE_DEFAULT = f"""{{}}"""

# Add to the top with other constants
CLASSIFICATION_STATS_FILE = TEST_OUTPUT_DIR / "classification-stats.json"

# Add new global variable for tracking
classification_stats = {
    "counts": defaultdict(int),
    "successes": defaultdict(int),
    "failures": defaultdict(int),
    "details": defaultdict(list)
}

@dataclass
class TestResult:
    chart_name: str
    chart_path: Path
    classification: str
    status: str # e.g., SUCCESS, TEMPLATE_ERROR, OVERRIDE_ERROR, UNKNOWN_ERROR
    category: str # Error category if status is an error
    details: str # Detailed error message
    override_duration: float
    validation_duration: float

def categorize_error(error_msg: str) -> str:
    """Categorize an error message based on keywords."""
    if not error_msg:
        return "UNKNOWN_ERROR"

    error_msg_lower = error_msg.lower()

    # Ordered by specificity / common patterns
    if "failed to copy" in error_msg_lower or "failed to fetch" in error_msg_lower or "429 too many requests" in error_msg_lower or "chart not found" in error_msg_lower:
        return "PULL_ERROR"
    if "path is not a directory or is missing" in error_msg_lower:
        return "SETUP_ERROR"
    if "returned non-zero exit status" in error_msg_lower and "helm-image-override override" in error_msg_lower:
         return "OVERRIDE_ERROR"
    if "override file not created or empty" in error_msg_lower:
        return "OVERRIDE_ERROR"
    if "failed to parse" in error_msg_lower and ("yaml:" in error_msg_lower or "json:" in error_msg_lower):
        return "YAML_ERROR"
    if "invalid yaml syntax" in error_msg_lower:
        return "YAML_ERROR"
    if "schema(s)" in error_msg_lower and ("don't meet the specifications" in error_msg_lower or "validation failed" in error_msg_lower):
        return "SCHEMA_ERROR"
    if "wrong type for value" in error_msg_lower:
        return "COALESCE_ERROR"
    if "coalesce.go" in error_msg_lower and "warning: destination for" in error_msg_lower:
        return "COALESCE_ERROR"
    if "required" in error_msg_lower and ("is required" in error_msg_lower or "cannot be empty" in error_msg_lower or "must be set" in error_msg_lower):
        return "REQUIRED_VALUE_ERROR"
    if "execution error at" in error_msg_lower and ("notes.txt" in error_msg_lower or "validations.yaml" in error_msg_lower):
        return "REQUIRED_VALUE_ERROR"
    if "library charts are not installable" in error_msg_lower:
        return "LIBRARY_ERROR"
    if "chart is deprecated" in error_msg_lower:
        return "DEPRECATED_ERROR"
    if "timeout expired" in error_msg_lower:
        return "TIMEOUT_ERROR"
    # Generic template error check should be broad but after specific ones
    if "template" in error_msg_lower and ("error" in error_msg_lower or "failed" in error_msg_lower):
        return "TEMPLATE_ERROR"

    # Less specific checks
    if "error" in error_msg_lower or "failed" in error_msg_lower:
        # Generic error if none of the above matched
        return "UNKNOWN_ERROR"

    return "UNKNOWN_ERROR"

def ensure_chart_cache():
    """Ensure chart cache directory exists."""
    CHART_CACHE_DIR.mkdir(parents=True, exist_ok=True)

def get_cached_chart(chart_name: str) -> Optional[Path]:
    """Check if chart exists in cache."""
    cached_charts = list(CHART_CACHE_DIR.glob(f"{chart_name}*.tgz"))
    return max(cached_charts, key=lambda x: x.stat().st_mtime) if cached_charts else None

async def pull_chart(chart: str, output_dir: Path) -> Tuple[bool, Optional[Path]]:
    """Pull and untar a Helm chart asynchronously."""
    chart_name = chart.split('/')[1] if '/' in chart else chart
    # Use a temporary directory for pulling to avoid conflicts if chart name isn't unique
    # We'll move the actual chart dir to the cache later if successful.
    temp_pull_dir = Path(tempfile.mkdtemp(prefix=f"helm-pull-{chart_name}-"))
    chart_dir: Optional[Path] = None
    success = False

    try:
        print(f"  Attempting pull: {chart} into {temp_pull_dir}")
        cmd = ["helm", "pull", chart, "--untar", "--untardir", str(temp_pull_dir)]
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE
        )
        stdout, stderr = await asyncio.wait_for(process.communicate(), timeout=120) # Add timeout

        stdout_str = stdout.decode().strip() if stdout else ""
        stderr_str = stderr.decode().strip() if stderr else ""

        if process.returncode == 0:
            # Find the actual chart directory within the temp pull path
            found_dir = None
            for item in temp_pull_dir.iterdir():
                # Helm untars into a directory named after the chart
                if item.is_dir() and item.name == chart_name and (item / "Chart.yaml").exists():
                    found_dir = item
                    break
                # Handle cases where untar dir might differ slightly (e.g., subcharts)
                elif item.is_dir() and (item / "Chart.yaml").exists():
                     # Fallback: take the first directory found with a Chart.yaml
                     # This might be less reliable if multiple charts are untarred unexpectedly
                     if found_dir is None:
                          print(f"    Warning: Found potential chart dir '{item.name}' mismatching '{chart_name}' for {chart}")
                          found_dir = item
            
            if found_dir:
                # Move the found chart directory to the permanent cache
                target_cache_path = output_dir / chart_name
                if target_cache_path.exists():
                     print(f"    Removing existing cache dir: {target_cache_path}")
                     shutil.rmtree(target_cache_path)
                
                shutil.move(str(found_dir), str(target_cache_path))
                print(f"  Successfully pulled and cached {chart} to {target_cache_path}")
                chart_dir = target_cache_path # Return the path in the actual cache
                success = True
            else:
                print(f"  Error: Could not find chart directory in {temp_pull_dir} after helm pull for {chart}")
                print(f"    Stdout: {stdout_str}")
                print(f"    Stderr: {stderr_str}")
                # Log contents for debugging
                try:
                     dir_contents = os.listdir(temp_pull_dir)
                     print(f"    Contents of {temp_pull_dir}: {dir_contents}")
                except OSError as list_err:
                     print(f"    Could not list contents of {temp_pull_dir}: {list_err}")

        else:
            print(f"  Error pulling chart {chart} (code {process.returncode}):")
            if stderr_str:
                 print(f"    Stderr: {stderr_str}")
            if stdout_str:
                 print(f"    Stdout: {stdout_str}")

    except asyncio.TimeoutError:
        print(f"  Timeout pulling chart {chart}")
    except Exception as e:
        print(f"  Unexpected error pulling chart {chart}: {e}")
        import traceback
        traceback.print_exc()
    finally:
        # Clean up the temporary pull directory
        if temp_pull_dir.exists():
             shutil.rmtree(temp_pull_dir)

    return success, chart_dir

def test_chart(chart: str, chart_dir: Path) -> bool:
    """Test a chart's template rendering."""
    chart_name = os.path.basename(chart)
    chart_path = chart_dir / chart_name
    stdout_file = TEST_OUTPUT_DIR / f"{chart_name}-analysis-stdout.txt"
    stderr_file = TEST_OUTPUT_DIR / f"{chart_name}-analysis-stderr.txt"
    default_values_file = Path(__file__).parent / "lib" / "default-values.yaml"
    
    print(f"Testing chart template: {chart}")
    
    try:
        result = subprocess.run(
            ["helm", "template", str(chart_path), "-f", str(default_values_file)],
            stdout=open(stdout_file, "w"),
            stderr=open(stderr_file, "w"),
            check=True
        )
        return True
    except subprocess.CalledProcessError:
        log_error(
            ANALYSIS_RESULTS_FILE,
            f"{chart}: Template rendering failed",
            stderr_file,
            stdout_file
        )
        return False

def update_classification_stats(chart_name: str, classification: str, success: bool):
    """Update classification statistics."""
    classification_stats["counts"][classification] += 1
    if success:
        classification_stats["successes"][classification] += 1
    else:
        classification_stats["failures"][classification] += 1
    classification_stats["details"][classification].append({
        "chart": chart_name,
        "success": success
    })

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
        }
    }
    
    with open(CLASSIFICATION_STATS_FILE, 'w') as f:
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

async def test_chart_override(chart_info, target_registry, helm_override_binary, session):
    """
    Tests override generation and helm template validation for a single chart.
    Assumes the chart path points to an already downloaded/extracted chart directory.
    """
    chart_name, chart_path = chart_info
    stdout_file = TEST_OUTPUT_DIR / f"{chart_name}-override-stdout.txt"
    stderr_file = TEST_OUTPUT_DIR / f"{chart_name}-override-stderr.txt"
    output_file = TEST_OUTPUT_DIR / f"{chart_name}-values.yaml"
    
    print(f"\nTesting chart override: {chart_name} at {chart_path}")
    
    try:
        # First check if helm-override-binary exists
        if not helm_override_binary.exists():
            error_msg = f"helm-image-override binary not found at {helm_override_binary}"
            print(f"Error: {error_msg}")
            return TestResult(chart_name, chart_path, "UNKNOWN", "UNKNOWN_ERROR", error_msg, "", 0, 0)

        # Get cached chart
        cached_chart = get_cached_chart(chart_name)
        if not cached_chart:
            error_msg = f"Chart {chart_name} not found in cache"
            print(f"Error: {error_msg}")
            return TestResult(chart_name, chart_path, "UNKNOWN", "UNKNOWN_ERROR", error_msg, "", 0, 0)

        # Create temporary directory for chart extraction
        temp_dir = Path(tempfile.mkdtemp())
        chart_path = temp_dir / chart_name
        
        try:
            # Extract the cached chart
            print(f"Extracting cached chart: {cached_chart}")
            with tarfile.open(cached_chart) as tar:
                tar.extractall(temp_dir)
                
                # Find the extracted directory (usually there's only one)
                extracted_items = list(temp_dir.iterdir())
                if not extracted_items:
                    raise Exception("No files found in extracted chart")
                    
                # The chart directory is usually the only directory in the extracted contents
                chart_path = extracted_items[0]
                if not chart_path.is_dir():
                    raise Exception("Extracted chart path is not a directory")

            # Get chart classification
            classification = get_chart_classification(chart_path)
            print(f"Using classification: {classification}")

            # Build and print the override command
            override_cmd = [
                str(helm_override_binary),
                "override",
                "--chart-path", str(chart_path),
                "--target-registry", target_registry,
                "--source-registries", "docker.io,quay.io,gcr.io,ghcr.io,k8s.gcr.io,registry.k8s.io",
                "--output-file", str(output_file)
            ]
            print("\nExecuting override command:")
            print(" ".join(override_cmd))

            # Run the override command
            print("Generating overrides...")
            start_time = time.time()
            result = subprocess.run(
                override_cmd,
                stdout=open(stdout_file, "w"),
                stderr=open(stderr_file, "w"),
                check=True
            )
            override_duration = time.time() - start_time
            
            # Capture override stdout/stderr for potential errors even on success code
            override_stdout_str = stdout.decode().strip() if stdout else ""
            override_stderr_str = stderr.decode().strip() if stderr else ""

            if result.returncode != 0:
                error_msg = f"helm-image-override failed (code {result.returncode}):\nStderr: {override_stderr_str}\nStdout: {override_stdout_str}"
                print(f"  Error generating overrides for {chart_name}.")
                category = categorize_error(override_stderr_str or override_stdout_str) # Prioritize stderr
                return TestResult(chart_name, chart_path, classification, "OVERRIDE_ERROR", category, error_msg, override_duration, 0)
            elif not output_file.exists() or output_file.stat().st_size == 0:
                # Include override tool's output in the error message
                error_msg = f"Override file not created or empty after successful run for {chart_name}.\nStderr: {override_stderr_str}\nStdout: {override_stdout_str}"
                print(f"  Error: {error_msg}")
                category = categorize_error(error_msg)
                return TestResult(chart_name, chart_path, classification, "OVERRIDE_ERROR", category, error_msg, override_duration, 0)
            else:
                print(f"  Override generation successful ({override_duration:.2f}s). Output: {output_file}")

            # --- Helm Template Validation ---
            print(f"  Validating with helm template for {chart_name}...")
            start_time = time.time()
            temp_dir = None
            template_stdout_str = ""
            template_stderr_str = ""
            try:
                temp_dir = Path(tempfile.mkdtemp(prefix="helm-values-"))
                temp_values_path = temp_dir / "temp_class_values.yaml"

                values_content = get_values_content(classification, target_registry)
                temp_values_path.write_text(values_content)

                cmd = [
                    "helm", "template", chart_name, str(chart_path),
                    "-f", str(temp_values_path),
                    "-f", str(output_file)
                ]
                # Use asyncio.create_subprocess_exec for helm template as well
                template_process = await asyncio.create_subprocess_exec(
                    *cmd,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.PIPE
                )
                # Ensure communication happens to capture output
                stdout, stderr = await asyncio.wait_for(template_process.communicate(), timeout=60) 
                validation_duration = time.time() - start_time
                
                template_stdout_str = stdout.decode().strip() if stdout else ""
                template_stderr_str = stderr.decode().strip() if stderr else ""

                if template_process.returncode != 0:
                    # Combine command, stderr, and stdout for detailed error
                    error_details = f"Helm template failed (code {template_process.returncode}):\nCommand: {' '.join(cmd)}\nStderr: {template_stderr_str}\nStdout: {template_stdout_str}" 
                    print(f"  Helm template validation failed for {chart_name}.")
                    # Categorize based on stderr primarily, fallback to stdout
                    category = categorize_error(template_stderr_str or template_stdout_str) 
                    return TestResult(chart_name, chart_path, classification, "TEMPLATE_ERROR", category, error_details, override_duration, validation_duration)
                else:
                     print(f"  Helm template validation successful ({validation_duration:.2f}s).")

            # Catch exceptions specifically around the helm template subprocess call
            except (asyncio.TimeoutError, Exception) as template_exc:
                 validation_duration = time.time() - start_time # Capture time until exception
                 # Attempt to include any captured output, even partial
                 error_msg = f"Error during helm template execution for {chart_name}: {template_exc}\nStderr: {template_stderr_str}\nStdout: {template_stdout_str}"
                 print(f"Error: {error_msg}")
                 category = categorize_error(template_stderr_str or str(template_exc))
                 status = "TIMEOUT_ERROR" if isinstance(template_exc, asyncio.TimeoutError) else "TEMPLATE_ERROR"
                 return TestResult(chart_name, chart_path, classification, status, category, error_msg, override_duration, validation_duration)
            finally:
                if temp_dir and temp_dir.exists():
                    shutil.rmtree(temp_dir, ignore_errors=True)

            # If template validation passes
            print(f"Success: Chart {chart_name} processed successfully.")
            return TestResult(chart_name, chart_path, classification, "SUCCESS", "", "", override_duration, validation_duration)

        except Exception as e:
            error_msg = f"Unexpected error processing chart {chart_name}: {str(e)}"
            print(f"Error: {error_msg}")
            import traceback
            traceback.print_exc() # Print full traceback for unexpected errors
            category = categorize_error(str(e)) # Categorize based on the exception string
            return TestResult(chart_name, chart_path, classification, "UNKNOWN_ERROR", category, error_msg, 0, 0)

    except asyncio.TimeoutError as e:
        # Ensure override duration is captured if timeout happened there
        if override_duration == 0: override_duration = time.time() - start_time 
        error_msg = f"Timeout expired processing chart {chart_name}: {e}"
        print(f"Error: {error_msg}")
        category = categorize_error(error_msg)
        return TestResult(chart_name, chart_path, classification, "TIMEOUT_ERROR", category, error_msg, override_duration, 0)
    except Exception as e:
        # Ensure override duration is captured if error happened there
        if override_duration == 0: override_duration = time.time() - start_time
        # Attempt to get stderr/stdout if available from override context
        error_details = f"Unexpected error processing chart {chart_name}: {str(e)}"
        # Add override output if relevant context is available (variables might not exist here)
        # error_details += f"\nOverride Stderr: {override_stderr_str if 'override_stderr_str' in locals() else 'N/A'}"
        # error_details += f"\nOverride Stdout: {override_stdout_str if 'override_stdout_str' in locals() else 'N/A'}"
        print(f"Error: {error_details}")
        import traceback
        traceback.print_exc() # Print full traceback for unexpected errors
        category = categorize_error(str(e)) # Categorize based on the exception string
        return TestResult(chart_name, chart_path, classification, "UNKNOWN_ERROR", category, error_details, override_duration, 0)

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
        "kubevela": "https://kubevela.github.io/charts"
    }
    
    # Add repositories sequentially to avoid race conditions
    for name, url in repos.items():
        try:
            print(f"Adding repository: {name}")
            subprocess.run(
                ["helm", "repo", "add", name, url, "--force-update"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=True
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
            check=True
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
        check=True
    )
    
    charts = []
    for line in result.stdout.splitlines()[1:]:  # Skip header line
        parts = line.split()
        if parts:
            charts.append(parts[0])
    
    return sorted(set(charts))

# Logging functions
def log_error(file_path: Path, message: str, error_file: Path = None, stdout_file: Path = None):
    """Log error details to file and detailed logs directory."""
    with open(file_path, "a") as f:
        f.write(f"{message}\n")
        f.write("Error details:\n")
        
        # Get error content
        error_content = "No error output captured"
        if error_file and os.path.exists(error_file) and os.path.getsize(error_file) > 0:
            with open(error_file, "r") as ef:
                error_content = ef.read()
        
        # Categorize error
        error_category = categorize_error(error_content)
        error_description = ERROR_CATEGORIES.get(error_category, "Unknown error")
        
        f.write(f"Error Category: {error_category}\n")
        f.write(f"Category Description: {error_description}\n")
        
        # Log the actual error
        f.write(f"{error_content}\n")
        
        if stdout_file and os.path.exists(stdout_file) and os.path.getsize(stdout_file) > 0:
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
        
        if stdout_file and os.path.exists(stdout_file) and os.path.getsize(stdout_file) > 0:
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
        "error_categories": error_counts
    }
    
    with open(SUMMARY_JSON_FILE, "w") as f:
        json.dump(summary, f, indent=4)

# Process a single chart
def process_chart(chart: str, chart_dir: Path, target_registry: str) -> Tuple[str, int, int]:
    """Process a single chart and return results."""
    temp_dir = tempfile.mkdtemp()
    success_file = os.path.join(temp_dir, "success")
    
    try:
        # Get chart from cache
        chart_name = os.path.basename(chart)
        cached_chart = get_cached_chart(chart_name)
        
        if not cached_chart:
            print(f"Error: Chart {chart} not found in cache. Please run pull-charts.py first.")
            return chart, 0, 0
            
        print(f"Using cached chart: {cached_chart}")
        
        # Extract the cached chart
        with tarfile.open(cached_chart) as tar:
            tar.extractall(temp_dir)
            
            # Find the extracted directory
            chart_dir = next(Path(temp_dir).iterdir())
            
            # Test chart template rendering
            analysis_success = 0
            if test_chart(chart, Path(temp_dir)):
                analysis_success = 1
            
            # Test chart override generation
            override_success = 0
            if test_chart_override(chart, Path(temp_dir), target_registry):
                override_success = 1
            
            return chart, analysis_success, override_success
            
    except Exception as e:
        print(f"Error processing chart {chart}: {e}")
        return chart, 0, 0
    finally:
        # Cleanup
        shutil.rmtree(temp_dir, ignore_errors=True)

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
        print(f"\nAnalyzing chart structure for: {chart_path}")
        
        # Check Chart.yaml for Bitnami dependency
        chart_yaml_path = chart_path / "Chart.yaml"
        if chart_yaml_path.exists():
            print("  Found Chart.yaml")
            with open(chart_yaml_path) as f:
                chart_data = yaml.safe_load(f)
                
                # Check for Bitnami dependency
                if "dependencies" in chart_data:
                    print("  Checking dependencies...")
                    for dep in chart_data["dependencies"]:
                        if dep.get("name") == "common" and "bitnami" in dep.get("repository", ""):
                            print("  → Detected Bitnami common library dependency")
                            return "BITNAMI"
                
                # Check chart name/description for scanner patterns
                chart_name = chart_data.get("name", "").lower()
                chart_desc = chart_data.get("description", "").lower()
                if any(term in chart_name or term in chart_desc for term in ["scanner", "security", "aqua", "trivy"]):
                    print("  → Detected scanner/security chart")
                    return "STANDARD_MAP"
                
                print("  No special dependencies or patterns found in Chart.yaml")

        # Analyze values.yaml structure
        values_yaml_path = chart_path / "values.yaml"
        if values_yaml_path.exists():
            print("  Found values.yaml")
            with open(values_yaml_path) as f:
                values_data = yaml.safe_load(f)
                
                if not isinstance(values_data, dict):
                    print(f"  Values data type: {type(values_data)}")
                    return "DEFAULT"
                
                # Check for scanner-specific patterns
                if any(key in values_data for key in ["scanner", "aqua", "trivy", "security"]):
                    print("  → Detected scanner/security configuration")
                    return "STANDARD_MAP"
                
                # Check for global registry pattern
                global_data = values_data.get("global", {})
                if isinstance(global_data, dict):
                    if "imageRegistry" in global_data:
                        print("  Found global.imageRegistry")
                        if any("bitnami" in str(v).lower() for v in global_data.values()):
                            print("  → Detected Bitnami global configuration")
                            return "BITNAMI"
                    
                # Check for standard map pattern
                image_data = values_data.get("image", {})
                if isinstance(image_data, dict):
                    print("  Found image map structure")
                    if "repository" in image_data:
                        print("  → Detected standard map pattern with repository")
                        return "STANDARD_MAP"
                elif isinstance(image_data, str):
                    print("  → Detected string-based image configuration")
                    return "STANDARD_STRING"
                else:
                    print(f"  Image data type: {type(image_data)}")
        else:
            print("  No values.yaml found")

    except Exception as e:
        print(f"  Warning: Error during chart classification: {e}")
        
    print("  → Using DEFAULT classification")
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
    else: # DEFAULT or other unforeseen
        template = VALUES_TEMPLATE_DEFAULT

    # Replace placeholder with actual target registry
    final_template = template.replace(TARGET_REGISTRY_PLACEHOLDER, target_registry)
    # print(f"---\nClassification: {classification}\nGenerated Values:\n{final_template}\n---") # Debugging comment fixed
    return final_template

async def main():
    """
    Main orchestration function.
    Finds charts (expecting them to be cached/downloaded), runs tests concurrently,
    and processes results.
    Note: Chart pulling is handled by pull-charts.py or relies on a populated cache.
    """
    start_total_time = time.time()
    parser = argparse.ArgumentParser(description="Test Helm chart override generation.")
    parser.add_argument("--target-registry", required=True, help="Target registry URL")
    parser.add_argument("--chart-filter", default=None, help="Regex pattern to filter chart names")
    parser.add_argument("--max-charts", type=int, default=None, help="Maximum number of charts to test")
    parser.add_argument("--skip-charts", default=None, help="Comma-separated list of charts to skip")
    parser.add_argument("--max-workers", type=int, default=os.cpu_count(), help="Maximum number of parallel workers")
    parser.add_argument("--no-cache", action="store_true", help="Disable chart caching")
    parser.add_argument("--update-repos", action="store_true", help="Force update Helm repositories")
    args = parser.parse_args()

    # Ensure output directories exist
    TEST_OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    DETAILED_LOGS_DIR.mkdir(parents=True, exist_ok=True)
    CHART_CACHE_DIR.mkdir(parents=True, exist_ok=True)

    helm_override_binary = BASE_DIR / "build" / "helm-image-override"
    if not helm_override_binary.exists():
        print(f"Error: helm-image-override binary not found at {helm_override_binary}. Please build it first.")
        sys.exit(1)

    print("Adding Helm repositories...")
    add_helm_repositories()

    if args.update_repos:
        print("Updating Helm repositories...")
        update_helm_repositories()

    print("Listing charts...")
    all_charts_full_names = list_charts()
    print(f"Found {len(all_charts_full_names)} total charts in repositories.")

    # Apply filtering
    charts_to_test_names = []
    skip_list = args.skip_charts.split(',') if args.skip_charts else []
    if args.chart_filter:
        pattern = re.compile(args.chart_filter)
        charts_to_test_names = [chart for chart in all_charts_full_names if pattern.search(chart) and chart not in skip_list]
        print(f"Filtered to {len(charts_to_test_names)} charts based on pattern: {args.chart_filter}")
    else:
        charts_to_test_names = [chart for chart in all_charts_full_names if chart not in skip_list]

    # Apply max charts limit
    if args.max_charts is not None and len(charts_to_test_names) > args.max_charts:
        charts_to_test_names = charts_to_test_names[:args.max_charts]
        print(f"Limited to testing {len(charts_to_test_names)} charts.")

    if not charts_to_test_names:
        print("No charts selected for testing after filtering.")
        sys.exit(0)

    # --- Chart Pulling (Now async) ---
    print("\n--- Ensuring Charts are Cached ---")
    pull_tasks = []
    charts_to_attempt_pull = []
    # Decide which charts need pulling
    for chart_full_name in charts_to_test_names:
        chart_name = chart_full_name.split('/')[1] if '/' in chart_full_name else chart_full_name
        cached_path = get_cached_chart(chart_name)
        if not cached_path or args.no_cache:
            charts_to_attempt_pull.append(chart_full_name)
        # else: Add chart info for cached charts later

    # Pull needed charts concurrently
    async with asyncio.Semaphore(10): # Limit concurrent helm pulls
        for chart_full_name in charts_to_attempt_pull:
             pull_tasks.append(asyncio.create_task(pull_chart(chart_full_name, CHART_CACHE_DIR)))
        
        pull_results = await asyncio.gather(*pull_tasks, return_exceptions=True)

    # Process pull results and gather charts to test
    charts_to_process_info: List[Tuple[str, Path]] = []
    pull_errors = 0
    pulled_chart_paths = {}
    for i, result in enumerate(pull_results):
        chart_full_name = charts_to_attempt_pull[i]
        if isinstance(result, Exception):
            print(f"Error pulling chart {chart_full_name}: {result}")
            pull_errors += 1
        elif result[0]: # Success
            chart_name = chart_full_name.split('/')[1] if '/' in chart_full_name else chart_full_name
            pulled_chart_paths[chart_name] = result[1]
        else: # Failure reported by pull_chart
            pull_errors += 1

    # Add successfully pulled and cached charts
    for chart_full_name in charts_to_test_names:
        chart_name = chart_full_name.split('/')[1] if '/' in chart_full_name else chart_full_name
        if chart_name in pulled_chart_paths:
            charts_to_process_info.append((chart_name, pulled_chart_paths[chart_name]))
        else:
            cached_path = get_cached_chart(chart_name)
            if cached_path and chart_name not in pulled_chart_paths: # Only add if not attempted/failed pull
                 charts_to_process_info.append((chart_name, cached_path))

    if pull_errors > 0:
        print(f"\nWarning: Failed to pull or process {pull_errors} charts.")
    if not charts_to_process_info:
        print("No charts available to test after pull/cache phase.")
        sys.exit(0)

    print(f"\n--- Starting Override and Validation for {len(charts_to_process_info)} charts ---")
    # --- Async Test Execution ---
    tasks = []
    # Limit concurrency using Semaphore
    # Use a dummy session; subprocess calls don't need a shared HTTP session
    limiter = asyncio.Semaphore(args.max_workers)
    async def run_with_limit(chart_info):
        async with limiter:
            return await test_chart_override(chart_info, args.target_registry, helm_override_binary, None)

    for chart_name, chart_path in charts_to_process_info:
        task = asyncio.create_task(run_with_limit((chart_name, chart_path)))
        tasks.append(task)

    # Wait for all test tasks to complete
    results: List[TestResult] = await asyncio.gather(*tasks, return_exceptions=True)

    # --- Process Results --- (Handle potential exceptions from gather)
    print("\n--- Processing Test Results ---")
    processed_results: List[TestResult] = []
    gather_errors = 0
    for result in results:
        if isinstance(result, TestResult):
            processed_results.append(result)
        elif isinstance(result, Exception):
            print(f"Error during task execution (likely asyncio gather): {result}")
            gather_errors += 1
            # Optionally create a dummy error result
            # processed_results.append(TestResult("UNKNOWN_CHART", Path("."), "UNKNOWN", "GATHER_ERROR", "UNKNOWN_ERROR", str(result), 0, 0))
        else:
             print(f"Warning: Unexpected item in gather results: {type(result)}")

    if gather_errors > 0:
        print(f"Warning: {gather_errors} tasks failed during asyncio.gather.")

    total_tested = len(processed_results)
    success_count = 0
    error_summary = defaultdict(int)
    error_details_rows = [] # For CSV
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
            error_details_rows.append({
                "Timestamp": datetime.now().isoformat(),
                "Chart": result.chart_name,
                "Classification": result.classification,
                "Status": result.status,
                "Category": category,
                "Details": result.details.strip()[:500] # Limit detail length
            })
            error_patterns[result.details.strip()] += 1
            update_classification_stats(result.chart_name, result.classification, False)

    # Save detailed error CSV
    error_details_file = TEST_OUTPUT_DIR / "error_details.csv"
    if error_details_rows:
        with open(error_details_file, 'w', newline='', encoding='utf-8') as csvfile:
            fieldnames = ["Timestamp", "Chart", "Classification", "Status", "Category", "Details"]
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()
            writer.writerows(error_details_rows)
        print(f"  - Error details: {error_details_file}")
    else:
        print("  - No errors to write to details file.")

    # Save error patterns
    error_patterns_file = TEST_OUTPUT_DIR / "error_patterns.txt"
    if error_patterns:
        with open(error_patterns_file, 'w', encoding='utf-8') as f:
            f.write("Error Message Patterns and Counts:\n")
            f.write("="*40 + "\n")
            # Sort by count descending
            sorted_patterns = sorted(error_patterns.items(), key=lambda item: item[1], reverse=True)
            for pattern, count in sorted_patterns:
                f.write(f"Count: {count}\n")
                f.write(f"Pattern:\n{pattern}\n")
                f.write("-"*40 + "\n")
        print(f"  - Error patterns: {error_patterns_file}")
    else:
        print("  - No error patterns to write.")

    # Save summary counts
    error_summary_file = TEST_OUTPUT_DIR / "error_summary.txt"
    with open(error_summary_file, 'w', encoding='utf-8') as f:
        f.write("Error Summary by Status Code:\n")
        f.write("="*40 + "\n")
        for status, count in error_summary.items():
            f.write(f"{status}: {count}\n")
    print(f"  - Error summary: {error_summary_file}")

    # Save classification stats
    save_classification_stats()
    print(f"  - Classification stats: {CLASSIFICATION_STATS_FILE}")

    # Final summary
    print("\n--- Summary ---")
    print(f"Total charts targeted: {len(charts_to_process_info)}")
    print(f"Total charts tested: {total_tested}")
    success_percentage = (success_count / total_tested * 100) if total_tested > 0 else 0
    print(f"Successful overrides: {success_count}/{total_tested} ({success_percentage:.0f}%)")
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
    asyncio.run(main()) # Run the async main function
