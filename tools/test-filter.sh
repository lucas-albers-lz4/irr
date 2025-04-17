#!/bin/bash
# test-filter.sh - A tool to run Go tests and filter output to show only relevant information
# Usage: ./tools/test-filter.sh [go test arguments]
# Example: ./tools/test-filter.sh ./... -v

# Set colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RESET='\033[0m'

# Display usage if no arguments provided
if [ "$#" -eq 0 ]; then
    echo -e "${YELLOW}Usage: $0 [go test arguments]${RESET}"
    echo -e "Example: $0 ./..."
    exit 1
fi

# Create a temporary file for storing the test output
TEMP_FILE=$(mktemp)

echo -e "${YELLOW}Running tests and capturing output...${RESET}"
go test "$@" | tee "$TEMP_FILE"

# Check if tests passed or failed
if grep -q "FAIL" "$TEMP_FILE"; then
    echo -e "\n${RED}=== TEST FAILURES DETECTED ===${RESET}"
    
    # Extract and display the test failures with context
    echo -e "\n${YELLOW}=== FAILURE DETAILS ===${RESET}"
    grep -B 1 -A 5 "--- FAIL:" "$TEMP_FILE"
    
    # Extract overall failed packages
    echo -e "\n${YELLOW}=== FAILED PACKAGES ===${RESET}"
    grep "^FAIL" "$TEMP_FILE"
    
    # Return failure exit code
    EXIT_CODE=1
else
    echo -e "\n${GREEN}=== ALL TESTS PASSED ===${RESET}"
    EXIT_CODE=0
fi

# Clean up
rm "$TEMP_FILE"

exit $EXIT_CODE 