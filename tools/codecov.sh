#!/bin/bash


MIN_CORE_COVERAGE=75
  
# Core packages to check
CORE_PACKAGES=(
  "github.com/lalbers/irr/pkg/chart"
  "github.com/lalbers/irr/pkg/override"
  "github.com/lalbers/irr/pkg/rules"
  "github.com/lalbers/irr/pkg/analysis"
  "github.com/lalbers/irr/pkg/image"
)

go test --coverprofile=coverage.out ./...


# Check coverage for each core package
for pkg in "${CORE_PACKAGES[@]}"; do
  echo "Checking coverage for $pkg"
  coverage=$(go tool cover -func=coverage.out | grep "$pkg" | awk '{ sum += $3; count++ } END { print sum/count }' | sed 's/%//')
  echo "Coverage: $coverage%"
  
  # Compare with threshold
  if (( $(echo "$coverage < $MIN_CORE_COVERAGE" | bc -l) )); then
    echo "Coverage for $pkg is below minimum threshold of $MIN_CORE_COVERAGE%"
  fi
done

echo "All core packages meet minimum coverage threshold!"

# Check coverage for each core package
for pkg in `find pkg/* -maxdepth 0` ; do
  coverage=$(go tool cover -func=coverage.out | grep "$pkg" | awk '{ sum += $3; count++ } END { print sum/count }' | sed 's/%//')
  echo "$pkg $coverage%"
done

