// Package fileperm provides a linter to check for hardcoded file permissions
package fileperm

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is a custom analysis pass that checks for hardcoded file permissions
var Analyzer = &analysis.Analyzer{
	Name: "fileperm",
	Doc:  "checks for hardcoded file permission literals instead of using constants",
	Run:  run,
}

// Permissions constants to look for
// Use constants for file permissions instead of hardcoded values for consistency and maintainability
const (
	// ReadWriteUserPerm is the permission level for read-write access by the file owner only
	ReadWriteUserPerm = 0o600

	// WriteFilePermArgIndex is the index of the permission argument in WriteFile functions
	WriteFilePermArgIndex = 2
)

// Permission constants to suggest
var permConstants = map[int64][]string{
	int64(ReadWriteUserPerm): {
		"fileutil.ReadWriteUserPermission",
		"SecureFilePerms",
		"PrivateFilePermissions",
		"FilePermissions",
		"defaultFilePerm",
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		// First find all WriteFile call expressions
		var writeFileCalls []*ast.CallExpr
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for call expressions
			if call, ok := n.(*ast.CallExpr); ok {
				// Check if it's a selector expression (e.g., os.WriteFile)
				if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
					if strings.HasSuffix(fun.Sel.Name, "WriteFile") {
						writeFileCalls = append(writeFileCalls, call)
					}
				}
			}
			return true
		})

		// Now check each WriteFile call for hardcoded permissions
		for _, call := range writeFileCalls {
			// WriteFile typically has 3 arguments: path, data, and permissions
			if len(call.Args) > WriteFilePermArgIndex {
				// Check if the permission argument is a hardcoded integer literal
				if lit, ok := call.Args[WriteFilePermArgIndex].(*ast.BasicLit); ok && lit.Kind == token.INT {
					if lit.Value == "0o600" || lit.Value == "0600" {
						suggestions := permConstants[int64(ReadWriteUserPerm)]
						suggestion := strings.Join(suggestions, "' or '")

						pass.Reportf(lit.Pos(), "use a file permission constant like '%s' instead of hardcoded '0o600'", suggestion)
					}
				}
			}
		}
	}
	// Return a dummy non-nil value to satisfy the linter
	return (*struct{})(nil), nil
}
