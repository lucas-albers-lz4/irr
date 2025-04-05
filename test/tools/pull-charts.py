#!/usr/bin/env python3

import argparse
import concurrent.futures
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Dict, List, Tuple, Any, Optional
import base64
import requests
from datetime import datetime, timedelta
from ratelimit import limits, sleep_and_retry

# Define configuration
BASE_DIR = Path(__file__).parent.parent.parent.absolute()
CHART_CACHE_DIR = BASE_DIR / "test" / "chart-cache"
PULL_LOGS_DIR = BASE_DIR / "test" / "pull-logs"

# Docker Hub rate limits (per 6 hours)
UNAUTHENTICATED_RATE_LIMIT = 100
AUTHENTICATED_RATE_LIMIT = 200
RATE_LIMIT_PERIOD = 6 * 60 * 60  # 6 hours in seconds

class RateLimitExceeded(Exception):
    """Exception raised when rate limit is exceeded."""
    pass

def check_docker_rate_limits(token: Optional[str] = None) -> Dict[str, Any]:
    """Check Docker Hub rate limit status."""
    headers = {}
    if token:
        headers['Authorization'] = f'Bearer {token}'
    
    try:
        response = requests.head(
            'https://registry-1.docker.io/v2/bitnamicharts/redis/manifests/latest',
            headers=headers
        )
        
        # Extract rate limit information from headers
        remaining = int(response.headers.get('RateLimit-Remaining', 0))
        limit = int(response.headers.get('RateLimit-Limit', UNAUTHENTICATED_RATE_LIMIT))
        reset_time = int(response.headers.get('RateLimit-Reset', 0))
        
        return {
            'remaining': remaining,
            'limit': limit,
            'reset': datetime.fromtimestamp(reset_time),
            'authenticated': bool(token)
        }
    except Exception as e:
        print(f"Warning: Failed to check rate limits: {e}")
        # Return conservative defaults
        return {
            'remaining': UNAUTHENTICATED_RATE_LIMIT,
            'limit': UNAUTHENTICATED_RATE_LIMIT,
            'reset': datetime.now() + timedelta(hours=6),
            'authenticated': bool(token)
        }

def calculate_rate_limit_delay(rate_info: Dict[str, Any]) -> float:
    """Calculate delay needed between requests based on rate limits."""
    remaining_time = (rate_info['reset'] - datetime.now()).total_seconds()
    if remaining_time <= 0:
        return 0
    
    # Add 10% buffer to be conservative
    safe_remaining = max(1, rate_info['remaining'] - (rate_info['remaining'] * 0.1))
    
    # Calculate delay needed to spread remaining requests over time
    return remaining_time / safe_remaining

@sleep_and_retry
@limits(calls=1, period=1)  # Base limit of 1 call per second
def rate_limited_pull(cmd: List[str], stdout_file: Path, stderr_file: Path, 
                     rate_info: Dict[str, Any]) -> subprocess.CompletedProcess:
    """Execute pull command with rate limiting."""
    # Calculate and apply dynamic rate limit delay
    delay = calculate_rate_limit_delay(rate_info)
    if delay > 0:
        time.sleep(delay)
    
    return subprocess.run(cmd, stdout=open(stdout_file, "w"), 
                        stderr=open(stderr_file, "w"), check=True)

def ensure_directories():
    """Ensure required directories exist."""
    CHART_CACHE_DIR.mkdir(parents=True, exist_ok=True)
    PULL_LOGS_DIR.mkdir(parents=True, exist_ok=True)

def get_cached_chart(chart_name: str) -> Optional[Path]:
    """Check if chart exists in cache."""
    cached_charts = list(CHART_CACHE_DIR.glob(f"{chart_name}*.tgz"))
    return max(cached_charts, key=lambda x: x.stat().st_mtime) if cached_charts else None

def get_docker_config() -> Optional[Dict[str, Any]]:
    """Get Docker config from standard location."""
    docker_config_path = Path.home() / ".docker" / "config.json"
    if docker_config_path.exists():
        with open(docker_config_path) as f:
            return json.load(f)
    return None

def encode_docker_auth(username: str, token: str) -> str:
    """Encode Docker auth credentials in base64."""
    auth_string = f"{username}:{token}"
    return base64.b64encode(auth_string.encode()).decode()

def get_docker_auth_token() -> Optional[Tuple[str, str]]:
    """Get Docker Hub authentication token and username from config or environment."""
    # First check environment variables
    token = os.environ.get("DOCKER_AUTH_TOKEN")
    username = os.environ.get("DOCKER_USERNAME", "")
    if token:
        return username, token
        
    # Then check Docker config file
    if docker_config := get_docker_config():
        auths = docker_config.get("auths", {})
        if "https://index.docker.io/v1/" in auths:
            auth_data = auths["https://index.docker.io/v1/"]
            return auth_data.get("username", ""), auth_data.get("auth")
    return None

def pull_chart(chart: str) -> bool:
    """Pull a chart from its repository."""
    chart_name = os.path.basename(chart)
    stdout_file = PULL_LOGS_DIR / f"{chart_name}-pull-stdout.txt"
    stderr_file = PULL_LOGS_DIR / f"{chart_name}-pull-stderr.txt"
    
    print(f"Processing chart: {chart}")
    
    # Check local cache first
    cached_chart = get_cached_chart(chart_name)
    if cached_chart:
        print(f"  ✓ Already cached: {cached_chart}")
        return True
    
    # Get Docker auth token and check rate limits
    docker_auth = get_docker_auth_token()
    token = docker_auth[1] if docker_auth else None
    rate_info = check_docker_rate_limits(token)
    
    if rate_info['remaining'] <= 0:
        reset_time = rate_info['reset'].strftime('%Y-%m-%d %H:%M:%S')
        print(f"  ⚠ Rate limit exceeded. Resets at {reset_time}")
        wait_time = (rate_info['reset'] - datetime.now()).total_seconds()
        if wait_time > 0:
            print(f"  Waiting {int(wait_time)} seconds for rate limit reset...")
            time.sleep(wait_time + 1)  # Add 1 second buffer
            rate_info = check_docker_rate_limits(token)  # Refresh rate info
    
    # Add retry logic for rate limits with increased delays
    max_retries = 3
    base_delay = 10  # seconds
    
    for attempt in range(max_retries):
        try:
            if attempt > 0:
                print(f"  Retry attempt {attempt + 1} for {chart}")
                retry_delay = base_delay * (2 ** attempt)  # Exponential backoff
                time.sleep(retry_delay)
                # Refresh rate limits after waiting
                rate_info = check_docker_rate_limits(token)
            
            # Try local mirror first (if configured)
            if os.environ.get("HELM_REGISTRY_MIRROR"):
                try:
                    mirror_cmd = [
                        "helm", "pull", chart,
                        "--destination", str(CHART_CACHE_DIR),
                        "--registry-config", os.environ["HELM_REGISTRY_MIRROR"]
                    ]
                    print("  Trying local mirror...")
                    rate_limited_pull(mirror_cmd, stdout_file, stderr_file, rate_info)
                    # If we get here, mirror succeeded
                    cached_chart = get_cached_chart(chart_name)
                    if cached_chart:
                        print(f"  ✓ Successfully pulled from mirror: {cached_chart}")
                        return True
                except subprocess.CalledProcessError:
                    print("  ✗ Mirror pull failed, falling back to Docker Hub")
            
            # Build pull command with authentication if available
            pull_cmd = [
                "helm", "pull", chart,
                "--destination", str(CHART_CACHE_DIR)
            ]
            
            if docker_auth:
                username, token = docker_auth
                encoded_auth = encode_docker_auth(username, token)
                registry_json = {
                    "auths": {
                        "https://index.docker.io/v1/": {
                            "auth": encoded_auth,
                            "username": username,
                            "password": token
                        }
                    }
                }
                # Create temporary registry config
                with tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False) as temp_config:
                    json.dump(registry_json, temp_config)
                    pull_cmd.extend(["--registry-config", temp_config.name])
                    print(f"  Using Docker Hub authentication (Rate limit: {rate_info['remaining']}/{rate_info['limit']} remaining)")
            
            # Pull the chart with rate limiting
            rate_limited_pull(pull_cmd, stdout_file, stderr_file, rate_info)
            
            # Clean up temp config if used
            if docker_auth:
                os.unlink(temp_config.name)
            
            # Verify the chart was downloaded
            cached_chart = get_cached_chart(chart_name)
            if cached_chart:
                print(f"  ✓ Successfully pulled: {cached_chart}")
                return True
            else:
                print(f"  ✗ Failed to find downloaded chart for {chart}")
                return False
            
        except subprocess.CalledProcessError:
            with open(stderr_file, "r") as f:
                error_content = f.read()
                if "429" in error_content or "toomanyrequests" in error_content:
                    # Refresh rate limits
                    rate_info = check_docker_rate_limits(token)
                    if rate_info['remaining'] <= 0:
                        reset_time = rate_info['reset'].strftime('%Y-%m-%d %H:%M:%S')
                        print(f"  ⚠ Rate limit exceeded. Resets at {reset_time}")
                        wait_time = (rate_info['reset'] - datetime.now()).total_seconds()
                        if wait_time > 0 and attempt < max_retries - 1:
                            print(f"  Waiting {int(wait_time)} seconds for rate limit reset...")
                            time.sleep(wait_time + 1)  # Add 1 second buffer
                            continue
                print(f"  ✗ Failed to pull chart: {error_content.strip()}")
            return False
        except Exception as e:
            print(f"  ✗ Error pulling chart: {str(e)}")
            return False

def add_helm_repositories():
    """Add Helm repositories."""
    print("\nAdding Helm repositories...")
    
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
            print(f"  Adding repository: {name}")
            subprocess.run(
                ["helm", "repo", "add", name, url, "--force-update"],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                check=True
            )
            # Add a small delay between adds to avoid overwhelming the server
            time.sleep(1)
        except subprocess.CalledProcessError:
            print(f"  ✗ Failed to add {name} repository")

def update_helm_repositories():
    """Update Helm repositories sequentially."""
    print("\nUpdating Helm repositories...")
    try:
        # Add a delay before update to prevent rate limits
        time.sleep(2)
        subprocess.run(
            ["helm", "repo", "update"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            check=True
        )
        print("  ✓ Successfully updated repositories")
        # Add a delay after update to prevent rate limits
        time.sleep(2)
    except subprocess.CalledProcessError:
        print("  ✗ Failed to update Helm repositories")

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

def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Pull and cache Helm charts")
    parser.add_argument("--chart-filter", help="Only process charts matching this pattern")
    parser.add_argument("--max-charts", type=int, help="Maximum number of charts to process")
    parser.add_argument("--skip-charts", help="Comma-separated list of charts to skip")
    parser.add_argument("--force", action="store_true", help="Force re-download of cached charts")
    parser.add_argument("--no-parallel", action="store_true", help="Disable parallel processing")
    parser.add_argument("--registry-mirror", help="Path to Helm registry mirror config")
    parser.add_argument("--docker-token", help="Docker Hub authentication token")
    parser.add_argument("--docker-username", help="Docker Hub username", default="")
    parser.add_argument("--rate-limit-delay", type=float, help="Additional delay between requests in seconds", default=0)
    args = parser.parse_args()

    # Set up environment variables if provided
    if args.registry_mirror:
        os.environ["HELM_REGISTRY_MIRROR"] = args.registry_mirror
    if args.docker_token:
        os.environ["DOCKER_AUTH_TOKEN"] = args.docker_token
    if args.docker_username:
        os.environ["DOCKER_USERNAME"] = args.docker_username

    # Check rate limits before starting
    token = os.environ.get("DOCKER_AUTH_TOKEN")
    rate_info = check_docker_rate_limits(token)
    print("\nDocker Hub Rate Limits:")
    print(f"  Remaining pulls: {rate_info['remaining']}/{rate_info['limit']}")
    print(f"  Reset time: {rate_info['reset'].strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"  Status: {'Authenticated' if rate_info['authenticated'] else 'Unauthenticated'}")
    
    # Calculate and show rate limiting strategy
    delay = calculate_rate_limit_delay(rate_info) + args.rate_limit_delay
    print(f"  Rate limiting delay: {delay:.2f} seconds between requests\n")

    # Ensure directories exist
    ensure_directories()

    # Add and update Helm repositories
    add_helm_repositories()
    update_helm_repositories()

    try:
        # Get list of charts
        print("\nGathering chart list...")
        charts = list_charts()
        
        # Apply chart filtering
        if args.chart_filter:
            pattern = re.compile(args.chart_filter)
            charts = [c for c in charts if pattern.search(c)]
            print(f"Filtered to {len(charts)} charts matching pattern: {args.chart_filter}")
        
        if args.skip_charts:
            skip_list = [s.strip() for s in args.skip_charts.split(",")]
            charts = [c for c in charts if c not in skip_list]
            print(f"Skipping {len(skip_list)} charts: {', '.join(skip_list)}")
        
        if args.max_charts and args.max_charts > 0:
            charts = charts[:args.max_charts]
            print(f"Limited to {args.max_charts} charts")
        
        print(f"\nFound {len(charts)} charts to process")
        
        # If force flag is set, clear the cache
        if args.force:
            print("\nForce flag set, clearing cache...")
            shutil.rmtree(CHART_CACHE_DIR)
            CHART_CACHE_DIR.mkdir()
        
        # Process charts based on execution mode
        successful_pulls = 0
        failed_pulls = 0
        
        if not args.no_parallel:
            print("\nPulling charts in parallel...")
            # Reduce number of parallel jobs to prevent rate limits
            max_workers = max(4, min(8, os.cpu_count() if os.cpu_count() else 4))
            print(f"Using {max_workers} worker processes")
            
            with concurrent.futures.ProcessPoolExecutor(max_workers=max_workers) as executor:
                futures = {executor.submit(pull_chart, chart): chart for chart in charts}
                
                for future in concurrent.futures.as_completed(futures):
                    chart = futures[future]
                    try:
                        if future.result():
                            successful_pulls += 1
                        else:
                            failed_pulls += 1
                    except Exception as e:
                        print(f"Error processing chart {chart}: {e}")
                        failed_pulls += 1
        else:
            print("\nPulling charts sequentially...")
            for chart in charts:
                if pull_chart(chart):
                    successful_pulls += 1
                else:
                    failed_pulls += 1
        
        # Print summary
        print("\nPull Summary:")
        total = len(charts)
        success_rate = (successful_pulls * 100 // total) if total > 0 else 0
        print(f"Successfully pulled: {successful_pulls}/{total} ({success_rate}%)")
        print(f"Failed pulls: {failed_pulls}/{total}")
        print(f"\nChart cache location: {CHART_CACHE_DIR}")
        print(f"Pull logs location: {PULL_LOGS_DIR}")
        
        return 0 if failed_pulls == 0 else 1
        
    except Exception as e:
        print(f"Error: {str(e)}")
        return 1

if __name__ == "__main__":
    sys.exit(main()) 