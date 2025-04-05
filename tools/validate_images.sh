#!/bin/bash
# tools/validate_images.sh

# Configuration
TARGET_REGISTRY=${1:-"localhost:5000"}
SOURCE_REGISTRIES=${2:-"docker.io,quay.io"}
CHART_PATH=${3:-"./tmp/chart"}
VALIDATE_PULL=${4:-"false"}

# Ensure chart exists
if [ ! -d "$CHART_PATH" ]; then
  echo "Error: Chart path $CHART_PATH does not exist"
  exit 1
fi

# Generate overrides
echo "Generating image overrides..."
./build/helm-image-override \
  --chart-path $CHART_PATH \
  --target-registry $TARGET_REGISTRY \
  --source-registries $SOURCE_REGISTRIES \
  --output-file ./tmp/overrides.yaml

# Extract all transformed images
echo "Extracting transformed images..."
helm template test $CHART_PATH -f ./tmp/overrides.yaml > ./tmp/rendered.yaml

# Extract image references
IMAGES=$(grep -o "$TARGET_REGISTRY/[^\"' ]*" ./tmp/rendered.yaml | sort | uniq)

# Save to file
echo "$IMAGES" > ./tmp/image_list.txt
echo "Found $(wc -l < ./tmp/image_list.txt) unique transformed images"

# Validate images exist if requested
if [ "$VALIDATE_PULL" = "true" ]; then
  echo "Validating images by pulling from registry..."
  SUCCESS=0
  FAILED=0
  
  while IFS= read -r image; do
    echo "Pulling: $image"
    if docker pull "$image"; then
      SUCCESS=$((SUCCESS+1))
      echo "✅ Successfully pulled $image"
    else
      FAILED=$((FAILED+1))
      echo "❌ Failed to pull $image"
    fi
  done < ./tmp/image_list.txt
  
  echo "Validation complete: $SUCCESS succeeded, $FAILED failed"
fi

echo "Image list saved to: ./tmp/image_list.txt"
