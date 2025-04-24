#!/bin/bash
# check-hardcoded-permissions.sh
# Checks for common hardcoded octal file/directory permissions

echo "Checking for common hardcoded octal permissions (e.g., 0o600, 0o644, 0o755)..."
echo

# Define the regex pattern for common octal permissions used with WriteFile/MkdirAll
# Looks specifically for the octal number as the last argument before the closing parenthesis.
PERMISSION_PATTERN='(WriteFile|MkdirAll)\(.*,\s*\b(0[o]*[67][0-7][0-7])\b\s*\)'

# Find all Go files calling WriteFile/MkdirAll with matching permissions, get line numbers (-n)
# Filter out constants definition and specific allowed files
hardcoded_perms_locations=$(find . -name "*.go" -type f -not \( -path "*/vendor/*" -o -path "./tools/lint/fileperm/*" \) -print0 | \
                            xargs -0 grep -nE "$PERMISSION_PATTERN" | \
                            grep -v "pkg/fileutil/constants.go" | \
                            grep  "pkg/chart/generator.go" |
                            grep  "cmd/irr/inspect.go" |
                            grep  "test/integration/harness.go" |
                            sed 's/^/   - /') # Indent results for clarity

# Get the list of defined constants from the source file to display to the user
# Assumes constants follow patterns like *Permission or *Permissions
DEFINED_CONSTANTS=$(grep -oE '\b[A-Z][a-zA-Z0-9]*(?:Permission|Permissions)\b' pkg/fileutil/constants.go | sed 's/^/     - fileutil./' | sort -u)

if [ -z "$hardcoded_perms_locations" ]; then
    echo "✅ No common hardcoded octal permissions found in WriteFile/MkdirAll calls!"
    exit 0
else
    echo "❌ Found hardcoded octal permissions in WriteFile/MkdirAll calls at:"
    echo "$hardcoded_perms_locations"
    echo
    echo "   Suggestion: Replace the hardcoded octal value (e.g., 0o644, 0o755) with an appropriate"
    echo "               semantic constant from 'pkg/fileutil'. Ensure the package is imported:"
    echo "               'github.com/lalbers/irr/pkg/fileutil'"
    echo
    echo "   Available constants in pkg/fileutil/constants.go:"
    if [ -n "$DEFINED_CONSTANTS" ]; then
        echo "$DEFINED_CONSTANTS"
    else
        echo "     (Could not automatically list constants)"
    fi
    echo
    echo "   Example (replace 0oXXX with the actual value found and ConstantName with the appropriate choice):"
    echo "   os.WriteFile(path, data, fileutil.ConstantName) // Use constants for permissions"
    echo "   os.MkdirAll(path, fileutil.ConstantName)        // Use constants for permissions"
    exit 1
fi 
