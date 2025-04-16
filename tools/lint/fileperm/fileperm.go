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
		ast.Inspect(file, func(n ast.Node) bool {
			// Look for basic literals (numbers)
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.INT {
				// Check if it's a file permission literal (0o600)
				if lit.Value == "0o600" || lit.Value == "0600" {
					// Check if this is a parameter to WriteFile functions
					found := false

					if call, ok := lit.Parent.(*ast.CallExpr); ok {
						if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
							fnName := fun.Sel.Name
							if strings.HasSuffix(fnName, "WriteFile") {
								found = true
							}
						}
					}

					if found {
						suggestions := permConstants[int64(ReadWriteUserPerm)]
						suggestion := strings.Join(suggestions, "' or '")

						pass.Reportf(lit.Pos(), "use a file permission constant like '%s' instead of hardcoded '0o600'", suggestion)
					}
				}
			}
			return true
		})
	}
	return nil, nil
}
