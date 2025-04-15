# Test Fixtures

This directory contains test fixtures used across the codebase to ensure consistent and reliable testing.

## Directory Structure

- `chart/` - Test fixtures for chart-related tests
- `rules/` - Test fixtures for rules system tests 
- `override/` - Test fixtures for override generation and manipulation
- `analysis/` - Test fixtures for analysis functionality
- `image/` - Test fixtures for image parsing and validation

## Usage Guidelines

1. **Organize by Package**: Place fixtures in the appropriate subdirectory matching the package they're primarily used for.
2. **JSON/YAML Preference**: Store structured data in JSON or YAML format for readability.
3. **Documentation**: Include a brief comment at the top of each fixture file explaining its purpose.
4. **Reuse**: Before creating new fixtures, check if existing ones can be reused or extended.
5. **Minimal Examples**: Keep fixtures focused on the specific test case they support.

## Loading Fixtures

Use the filesystem abstraction pattern when loading fixtures in tests:

```go
func TestWithFixtures(t *testing.T) {
    // Create mock filesystem
    mockFs := afero.NewMemMapFs()
    
    // Copy fixture to mock filesystem
    fixtureData, err := os.ReadFile("../testutil/fixtures/chart/basic_chart.yaml")
    if err != nil {
        t.Fatalf("Failed to read fixture: %v", err)
    }
    
    afero.WriteFile(mockFs, "chart.yaml", fixtureData, 0644)
    
    // Replace package filesystem with mock
    restore := somepackage.SetFs(mockFs)
    defer restore()
    
    // Run test with fixture
    // ...
}
``` 