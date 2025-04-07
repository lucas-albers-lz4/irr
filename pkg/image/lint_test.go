// Package image_test contains tests related to linting and validation within the image package.
package image

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDuplicateErrorDefinitions scans the package's Go files to ensure
// that no package-level error variable (identified by the `Err` prefix)
// is defined more than once.
func TestNoDuplicateErrorDefinitions(t *testing.T) {
	pkgDir := "." // Assumes the test is run within the pkg/image directory
	fset := token.NewFileSet()
	// Use a map to store variable names and their definition locations (filename:line)
	errorDefs := make(map[string][]string)

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("Failed to read package directory '%s': %v", pkgDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue // Skip directories, non-Go files, and test files
		}

		filePath := filepath.Join(pkgDir, entry.Name())
		f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("Failed to parse file '%s': %v", filePath, err)
		}

		// Inspect the AST for top-level var declarations
		ast.Inspect(f, func(n ast.Node) bool {
			decl, ok := n.(*ast.GenDecl)
			// Check if it's a var declaration at package level
			if !ok || decl.Tok != token.VAR {
				return true // Continue inspecting
			}

			// Iterate over variable specifications in the declaration (e.g., var Err1, Err2 = ...)
			for _, spec := range decl.Specs {
				vSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				// Iterate over names in the spec (e.g., Err1, Err2)
				for _, ident := range vSpec.Names {
					// Check if the variable name starts with "Err"
					if strings.HasPrefix(ident.Name, "Err") {
						pos := fset.Position(ident.Pos())
						location := fmt.Sprintf("%s:%d", filepath.Base(pos.Filename), pos.Line)
						errorDefs[ident.Name] = append(errorDefs[ident.Name], location)
					}
				}
			}
			return true // Continue inspecting
		})
	}

	// Check for duplicates
	foundDuplicates := false
	for errName, locations := range errorDefs {
		if len(locations) > 1 {
			foundDuplicates = true
			t.Errorf("Duplicate package-level error definition found for '%s':\n"+
				"\tDefined at: %s\n"+
				"\tRecommendation: Consolidate all package errors into errors.go and remove duplicates.",
				errName, strings.Join(locations, "\n\t            "))
		}
	}

	if foundDuplicates {
		t.Log("------\n" +
			"Tip: Ensure all package-level errors (variables starting with 'Err') " +
			"are defined only once, preferably in 'errors.go'.\n" +
			"------")
	}
}
