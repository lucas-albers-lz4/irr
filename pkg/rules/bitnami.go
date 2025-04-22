package rules

import (
	"strings"

	"github.com/lalbers/irr/pkg/log"
	"helm.sh/helm/v3/pkg/chart"
)

// BitnamiSecurityBypassPriority is the priority assigned to the Bitnami security bypass rule.
// Higher numbers mean higher priority.
const BitnamiSecurityBypassPriority = 100

// BitnamiSecurityBypassRule implements a rule that adds global.security.allowInsecureImages=true
// to override files for Bitnami charts
type BitnamiSecurityBypassRule struct {
	BaseRule
}

// NewBitnamiSecurityBypassRule creates a new BitnamiSecurityBypassRule
func NewBitnamiSecurityBypassRule() *BitnamiSecurityBypassRule {
	return &BitnamiSecurityBypassRule{
		BaseRule: NewBaseRule(
			"bitnami-security-bypass",
			"Adds global.security.allowInsecureImages=true to override files for Bitnami charts",
			[]Parameter{
				{
					Path:        "global.security.allowInsecureImages",
					Value:       true,
					Type:        TypeDeploymentCritical,
					Description: "Bypasses Bitnami security checks for modified image references",
				},
			},
			BitnamiSecurityBypassPriority,
		),
	}
}

// AppliesTo determines if this rule applies to a given chart
func (r *BitnamiSecurityBypassRule) AppliesTo(ch *chart.Chart) (Detection, bool) {
	// Use the common detection system
	detection := detectBitnamiChart(ch)

	// Only apply for medium or high confidence
	if detection.Confidence >= ConfidenceMedium {
		log.Debug("Bitnami security bypass rule applies to chart", "chart", ch.Name())
		return detection, true
	}

	return detection, false
}

// BitnamiFallbackHandler provides a fallback mechanism for handling Bitnami charts
// that fail with exit code 16 and specific error messages
type BitnamiFallbackHandler struct {
}

// NewBitnamiFallbackHandler creates a new BitnamiFallbackHandler
func NewBitnamiFallbackHandler() *BitnamiFallbackHandler {
	return &BitnamiFallbackHandler{}
}

// ShouldRetryWithSecurityBypass analyzes the error to determine if it's a Bitnami security error
// that could be fixed by adding global.security.allowInsecureImages=true
func (h *BitnamiFallbackHandler) ShouldRetryWithSecurityBypass(err error) bool {
	if err == nil {
		return false
	}

	// Convert error to string for analysis
	errStr := strings.ToLower(err.Error())

	// Check for both the exit code 16 and the specific error message patterns
	hasExitCode16 := strings.Contains(errStr, "exit code 16")
	hasSubstitutedContainers := strings.Contains(errStr, "original containers have been substituted for unrecognized ones")
	hasNonStandardContainers := strings.Contains(errStr, "if you are sure you want to proceed with non-standard containers")
	hasAllowInsecureImages := strings.Contains(errStr, "global.security.allowinsecureimages")

	// All patterns must be present
	return hasExitCode16 && hasSubstitutedContainers && hasNonStandardContainers && hasAllowInsecureImages
}

// ApplySecurityBypass adds the global.security.allowInsecureImages=true parameter to overrides
func (h *BitnamiFallbackHandler) ApplySecurityBypass(overrides map[string]interface{}) error {
	// Use the existing setValueAtPath function to set the parameter
	return setValueAtPath(overrides, "global.security.allowInsecureImages", true)
}
