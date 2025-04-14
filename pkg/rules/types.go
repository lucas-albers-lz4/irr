// Package rules provides a system for applying chart-specific rules and parameters
// to support successful Helm chart deployment with image registry overrides.
package rules

// ParameterType defines the type of parameter in terms of its criticality
type ParameterType int

const (
	// TypeDeploymentCritical (Type 1) represents parameters that MUST be in the
	// override file for successful chart deployment after applying IRR overrides.
	TypeDeploymentCritical ParameterType = 1

	// TypeTestValidationOnly (Type 2) represents parameters that are needed ONLY
	// for testing or validation but should NOT be in the final override file.
	TypeTestValidationOnly ParameterType = 2
)

// ConfidenceLevel indicates the confidence in a chart provider detection
type ConfidenceLevel int

const (
	// ConfidenceHigh indicates high confidence in the detection (multiple indicators)
	ConfidenceHigh ConfidenceLevel = 3

	// ConfidenceMedium indicates medium confidence (single strong indicator)
	ConfidenceMedium ConfidenceLevel = 2

	// ConfidenceLow indicates low confidence (weak indicators or fallback detection)
	ConfidenceLow ConfidenceLevel = 1

	// ConfidenceNone indicates no confidence in the detection
	ConfidenceNone ConfidenceLevel = 0
)

// ChartProviderType represents the type of chart provider/maintainer
type ChartProviderType string

const (
	// ProviderBitnami indicates a Bitnami chart
	ProviderBitnami ChartProviderType = "bitnami"

	// ProviderVMware indicates a VMware/Tanzu chart
	ProviderVMware ChartProviderType = "vmware"

	// ProviderStandard indicates a standard/common chart
	ProviderStandard ChartProviderType = "standard"

	// ProviderUnknown indicates an unknown chart provider
	ProviderUnknown ChartProviderType = "unknown"
)

// Parameter represents a chart parameter with its value and type
type Parameter struct {
	// Path is the dot-notation path to the parameter (e.g., "global.security.allowInsecureImages")
	Path string `json:"path" yaml:"path"`

	// Value is the value to set for the parameter
	Value interface{} `json:"value" yaml:"value"`

	// Type indicates whether this is a deployment-critical or test-only parameter
	Type ParameterType `json:"type" yaml:"type"`

	// Description provides context about the parameter's purpose
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Detection represents the result of chart provider detection
type Detection struct {
	// Provider is the detected chart provider type
	Provider ChartProviderType `json:"provider" yaml:"provider"`

	// Confidence is the confidence level in the detection
	Confidence ConfidenceLevel `json:"confidence" yaml:"confidence"`

	// Indicators contains the reasons for the detection
	Indicators []string `json:"indicators,omitempty" yaml:"indicators,omitempty"`
}
