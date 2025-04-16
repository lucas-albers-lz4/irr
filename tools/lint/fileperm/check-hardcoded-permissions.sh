#!/bin/bash
# check-hardcoded-permissions.sh
# Checks for hardcoded file permissions that should use constants

echo "Checking for hardcoded file permissions (0o600/0600)..."
echo

# Find all Go files with 0o600 and filter out constants
hardcoded_perms=$(find . -name "*.go" -type f | grep -v "/vendor/" | xargs grep -l "\s0o*600" | 
                  xargs grep -l "WriteFile" | 
                  grep -v "pkg/fileutil/constants.go" | 
                  grep -v "pkg/chart/generator.go" |
                  grep -v "cmd/irr/inspect.go" |
                  grep -v "test/integration/harness.go" |
                  grep -v "tools/lint/fileperm")

if [ -z "$hardcoded_perms" ]; then
    echo "✅ No hardcoded file permissions found!"
    exit 0
else
    echo "❌ Found hardcoded file permissions. Use constants instead:"
    echo "   - fileutil.ReadWriteUserPermission"
    echo "   - SecureFilePerms"
    echo "   - PrivateFilePermissions" 
    echo "   - FilePermissions"
    echo "   - defaultFilePerm"
    echo
    echo "Files with hardcoded permissions:"
    echo "$hardcoded_perms"
    echo "add comment to code direct to use constants (see that comment on next line)"
    echo "// Use constants for file permissions instead of hardcoded values for consistency and maintainability"
    exit 1
fi 
