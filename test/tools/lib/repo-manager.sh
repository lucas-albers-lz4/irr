#!/usr/bin/env bash

# Repository management module for test-charts.sh
# Handles all Helm repository operations

# Ensure strict mode
set -euo pipefail

# Source configuration
# shellcheck source=./config.sh
source "$(dirname "${BASH_SOURCE[0]}")/config.sh"

# Add a single Helm repository
add_single_repo() {
    local repo_name="$1"
    local repo_url="$2"
    
    if ! helm repo add "${repo_name}" "${repo_url}" > /dev/null 2>&1; then
        echo "Warning: Failed to add ${repo_name} repository"
    fi
}

# Add Helm repositories
add_helm_repositories() {
    echo "Adding Helm repositories..."
    
    # Add repositories in parallel
    add_single_repo "bitnami" "https://charts.bitnami.com/bitnami" &
    add_single_repo "ingress-nginx" "https://kubernetes.github.io/ingress-nginx" &
    add_single_repo "prometheus-community" "https://prometheus-community.github.io/helm-charts" &
    add_single_repo "grafana" "https://grafana.github.io/helm-charts" &
    add_single_repo "fluxcd" "https://fluxcd-community.github.io/helm-charts" &
    
    # Wait for all repo additions to complete
    wait
}

# Update Helm repositories
update_helm_repositories() {
    echo "Updating Helm repositories..."
    # Update repositories sequentially to avoid conflicts
    if ! helm repo update > /dev/null 2>&1; then
        echo "Warning: Failed to update Helm repositories"
    fi
}

# List available charts from all repositories
list_charts() {
    # Get a list of all charts from all repositories
    helm search repo -l | tail -n +2 | awk '{print $1}' | sort -u | while read -r chart; do
        # Extract repo and chart name
        repo=$(echo "${chart}" | cut -d'/' -f1)
        name=$(echo "${chart}" | cut -d'/' -f2)
        echo "${repo}/${name}"
    done
}

# Export functions
export -f add_single_repo
export -f add_helm_repositories
export -f update_helm_repositories
export -f list_charts 