# File Permission Linter

This custom linter checks for hardcoded file permissions in Go code and suggests using constants instead.

## Background

Hardcoded file permissions like `0o600` should be replaced with constants for better code consistency and maintainability. The project has several predefined constants for file permissions:

- `fileutil.ReadWriteUserPermission` (in pkg/fileutil/constants.go)
- `SecureFilePerms` (in cmd/irr/inspect.go)
- `PrivateFilePermissions` and `FilePermissions` (in pkg/chart/generator.go)
- `defaultFilePerm` (in test/integration/harness.go)

## Usage

### Using the Shell Script

Run the shell script directly:

```bash
./tools/lint/fileperm/check-hardcoded-permissions.sh
```

Or use the Makefile target:

```bash
make lint-fileperm
```

### Fixing Issues

When hardcoded permissions are found, update the code to use the appropriate constant based on the package:

```go
// Before:
err = os.WriteFile(filePath, data, 0o600)

// After:
import "github.com/lalbers/irr/pkg/fileutil"

err = os.WriteFile(filePath, data, fileutil.ReadWriteUserPermission)
```

## Rationale

Using constants for file permissions provides several benefits:

1. **Consistency** - All code uses the same permission values
2. **Maintainability** - Permissions can be updated in one place if needed
3. **Documentation** - Constants provide semantic meaning to the permission values
4. **Static Analysis** - Easier to verify security standards are followed

## Future Improvements

The custom linter could be extended to:

1. Add support for checking other permission values (0o644, 0o755, etc.)
2. Implement a proper Go analyzer plugin for golangci-lint
3. Auto-fix capabilities via a separate tool 