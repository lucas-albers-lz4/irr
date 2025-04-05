#!/bin/bash
# test/tools/top50_test.sh

CHARTS=(
  "bitnami/wordpress"
  "bitnami/nginx"
  "bitnami/mysql"
  # Add remaining from top 50 charts
)

TARGET_REGISTRY="test-registry.example.com"
SOURCE_REGISTRIES="docker.io,quay.io,gcr.io,ghcr.io"

for chart in "${CHARTS[@]}"; do
  echo "Testing chart: $chart"
  # Add helm repo if needed
  helm repo add --force-update $(echo $chart | cut -d/ -f1) https://charts.$(echo $chart | cut -d/ -f1).com/
  
  # Pull chart locally
  helm pull $chart --untar --destination ./tmp
  
  # Get chart directory
  chart_dir="./tmp/$(echo $chart | cut -d/ -f2)"
  
  # Run tool
  ./build/helm-image-override \
    --chart-path $chart_dir \
    --target-registry $TARGET_REGISTRY \
    --source-registries $SOURCE_REGISTRIES \
    --output-file ./tmp/overrides.yaml \
    --verbose
    
  # Validate with helm template
  if helm template test $chart_dir -f ./tmp/overrides.yaml > /dev/null; then
    echo "✅ Chart template validated successfully"
  else
    echo "❌ Chart template validation failed"
  fi
  
  # Add more validation as needed
  
  # Cleanup
  rm -rf $chart_dir ./tmp/overrides.yaml
done
