package version

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lalbers/irr/pkg/exitcodes"
	log "github.com/lalbers/irr/pkg/log"
)

const (
	// MinHelmVersion is the minimum required Helm version
	MinHelmVersion = "3.14.0"
)

// CheckHelmVersion checks if the installed Helm version meets our requirements
func CheckHelmVersion() error {
	// Get Helm version
	cmd := exec.Command("helm", "version", "--short")
	output, err := cmd.Output()
	if err != nil {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to get Helm version: %w", err),
		}
	}

	// Parse version string (e.g., "v3.14.2+g0e1f115")
	version := strings.TrimSpace(string(output))
	version = strings.TrimPrefix(version, "v")
	version = strings.Split(version, "+")[0]

	// Compare versions
	if !isVersionGreaterOrEqual(version, MinHelmVersion) {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("Helm version %s is not supported. Minimum required version is %s", version, MinHelmVersion),
		}
	}

	log.Debugf("Helm version check passed: %s", version)
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
		fmt.Sscanf(v1Parts[i], "%d", &v1Num)
		fmt.Sscanf(v2Parts[i], "%d", &v2Num)

		if v1Num > v2Num {
			return true
		}
		if v1Num < v2Num {
			return false
		}
	}

	return true
}
