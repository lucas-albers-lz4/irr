package helm

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"helm.sh/helm/v3/pkg/cli"
)

const (
	// MinHelmVersion is the minimum supported Helm version
	MinHelmVersion = "3.10.0"
	// MaxHelmVersion is the maximum supported Helm version
	MaxHelmVersion = "3.12.0"
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

// CheckHelmVersion checks if the installed Helm version is compatible
func CheckHelmVersion(settings *cli.EnvSettings) error {
	version, err := GetHelmVersion()
	if err != nil {
		return fmt.Errorf("failed to get Helm version: %w", err)
	}

	major, minor, patch, err := ParseHelmVersion(version)
	if err != nil {
		return fmt.Errorf("failed to parse Helm version: %w", err)
	}

	// Check if version is too old
	minMajor, minMinor, minPatch, err := ParseHelmVersion(MinHelmVersion)
	if err != nil {
		return fmt.Errorf("failed to parse minimum Helm version: %w", err)
	}

	if major < minMajor || (major == minMajor && minor < minMinor) || (major == minMajor && minor == minMinor && patch < minPatch) {
		return fmt.Errorf("Helm version %s is not supported. Please upgrade to the latest version", version)
	}

	// Check if version is too new
	maxMajor, maxMinor, maxPatch, err := ParseHelmVersion(MaxHelmVersion)
	if err != nil {
		return fmt.Errorf("failed to parse maximum Helm version: %w", err)
	}

	if major > maxMajor || (major == maxMajor && minor > maxMinor) || (major == maxMajor && minor == maxMinor && patch > maxPatch) {
		return fmt.Errorf("Helm version %s is not supported. Please downgrade to the latest version", version)
	}

	return nil
}

// ParseHelmVersion parses a Helm version string into major, minor, and patch versions
func ParseHelmVersion(version string) (int, int, int, error) {
	// Remove v prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split version into major, minor, patch
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", version)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %s", parts[0])
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %s", parts[1])
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %s", parts[2])
	}

	return major, minor, patch, nil
}
