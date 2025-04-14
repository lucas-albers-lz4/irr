// Package helm provides utilities for version checking and comparison,
// particularly for validating Helm version requirements.
package helm

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const (
	// MinHelmVersion is the minimum supported Helm version
	MinHelmVersion = "3.10.0"
	// MaxHelmVersion is the maximum supported Helm version
	MaxHelmVersion = "3.12.0"
	minSemverParts = 3 // minimum number of parts in semantic version (major.minor.patch)
)

// GetHelmVersion is a variable holding the function to get Helm version
// This allows for mocking in tests
var GetHelmVersion = func() (string, error) {
	cmd := exec.Command("helm", "version", "--short")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Helm version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")
	return version, nil
}

// CheckHelmVersion checks if the Helm version is compatible
func CheckHelmVersion() error {
	// Get Helm version
	version, err := GetHelmVersion()
	if err != nil {
		return err
	}

	// Parse major, minor, patch
	major, minor, _, err := ParseHelmVersion(version)
	if err != nil {
		return err
	}

	// Check if version is compatible
	if major < 3 || (major == 3 && minor < 10) {
		return fmt.Errorf("helm version %s is not supported. Please upgrade to the latest version", version)
	}

	// Check if version is too new
	if major > 3 || (major == 3 && minor > 12) {
		return fmt.Errorf("helm version %s is not supported. Please downgrade to the latest version", version)
	}

	return nil
}

// ParseHelmVersion parses a helm version string and returns the major, minor, and patch versions
func ParseHelmVersion(version string) (major, minor, patch int, err error) {
	const requiredSemverParts = 3

	parts := strings.Split(version, ".")
	if len(parts) < requiredSemverParts {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", version)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("unable to parse major version '%s' as int: %w", parts[0], err)
	}

	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("unable to parse minor version '%s' as int: %w", parts[1], err)
	}

	patch, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("unable to parse patch version '%s' as int: %w", parts[2], err)
	}

	return major, minor, patch, nil
}
