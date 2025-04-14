package rules

import (
	"github.com/lalbers/irr/pkg/debug"
	"helm.sh/helm/v3/pkg/chart"
)

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
			100, // High priority
		),
	}
}

// AppliesTo determines if this rule applies to a given chart
func (r *BitnamiSecurityBypassRule) AppliesTo(ch *chart.Chart) (Detection, bool) {
	// Use the common detection system
	detection := detectBitnamiChart(ch)

	// Only apply for medium or high confidence
	if detection.Confidence >= ConfidenceMedium {
		debug.Printf("Bitnami security bypass rule applies to chart: %s", ch.Name())
		return detection, true
	}

	return detection, false
}

// BitnamiFallbackHandler provides a fallback mechanism for handling Bitnami charts
// that fail with exit code 16 and specific error messages
type BitnamiFallbackHandler struct {
	// TODO: Implement fallback detection based on exit code 16 and error messages
	// This would be implemented in the validate command post-failure handling
	// For now, we focus on the metadata-based detection which is proactive rather than reactive
}
