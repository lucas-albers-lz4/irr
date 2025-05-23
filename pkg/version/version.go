// Package version provides utilities for version checking and comparison,
// particularly for validating Helm version requirements.
package version

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	// MinHelmVersion is the minimum required Helm version
	MinHelmVersion = "3.14.0"
)

// Variable for exec.Command to support mocking in tests
var execCommand = exec.Command

// parseHelmVersionString extracts the core semantic version (e.g., "3.14.2")
// from the typical output of `helm version --short` (e.g., "v3.14.2+g0e1f115").
// It removes the leading 'v' and any build metadata suffix starting with '+'.
func parseHelmVersionString(versionStr string) string {
	parsed := strings.TrimSpace(versionStr)
	parsed = strings.TrimPrefix(parsed, "v")
	parts := strings.Split(parsed, "+")
	// Ensure the split returned at least one element before accessing index 0
	if len(parts) > 0 {
		parsed = parts[0]
	} else {
		// This case should be impossible with strings.Split, but handle defensively.
		log.Warn("strings.Split unexpectedly returned an empty slice for version string part", "part", parsed)
		parsed = "" // Return empty string or handle error as appropriate
	}
	return parsed
}

// CheckHelmVersion checks if the installed Helm version meets our requirements
func CheckHelmVersion() error {
	// Get Helm version
	cmd := execCommand("helm", "version", "--short")
	output, err := cmd.Output()
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get Helm version: %w", err),
		}
	}

	// Parse version string
	version := parseHelmVersionString(string(output))

	// Compare versions
	if !isVersionGreaterOrEqual(version, MinHelmVersion) {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("helm version %s is not supported. Minimum required version is %s", version, MinHelmVersion),
		}
	}

	log.Debug("Helm version check passed", "version", version)
	return nil
}

// isVersionGreaterOrEqual compares two semantic versions
func isVersionGreaterOrEqual(v1, v2 string) bool {
	// Split versions into components
	v1Parts := strings.Split(v1, ".")
	v2Parts := strings.Split(v2, ".")

	// Compare each component
	for i := 0; i < 3; i++ {
		if i >= len(v1Parts) || i >= len(v2Parts) {
			return false
		}

		v1Num := 0
		v2Num := 0
		if _, err := fmt.Sscanf(v1Parts[i], "%d", &v1Num); err != nil {
			// If we can't parse the version number, treat it as 0
			v1Num = 0
		}
		if _, err := fmt.Sscanf(v2Parts[i], "%d", &v2Num); err != nil {
			// If we can't parse the version number, treat it as 0
			v2Num = 0
		}

		if v1Num > v2Num {
			return true
		}
		if v1Num < v2Num {
			return false
		}
	}

	return true
}
