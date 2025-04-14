package rules

import (
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"helm.sh/helm/v3/pkg/chart"
)

const bitnamiMediumConfidenceIndicatorCount = 2

// DetectChartProvider analyzes chart metadata to determine the provider type
// and returns a Detection with confidence level and supporting indicators
func DetectChartProvider(ch *chart.Chart) Detection {
	if ch == nil || ch.Metadata == nil {
		return Detection{
			Provider:   ProviderUnknown,
			Confidence: ConfidenceNone,
		}
	}

	// Try Bitnami detection first
	bitnamiDetection := detectBitnamiChart(ch)
	if bitnamiDetection.Confidence > ConfidenceNone {
		return bitnamiDetection
	}

	// Add other provider detections here later
	// e.g. detectVMwareChart(ch), detectStandardChart(ch), etc.

	// Default to unknown
	return Detection{
		Provider:   ProviderUnknown,
		Confidence: ConfidenceNone,
	}
}

// detectBitnamiChart checks if a chart is from Bitnami/Broadcom
// using tiered confidence detection
func detectBitnamiChart(ch *chart.Chart) Detection {
	indicators := []string{}
	metadata := ch.Metadata

	// Check direct indicators

	// 1. Check home field for bitnami.com
	if metadata.Home != "" && strings.Contains(strings.ToLower(metadata.Home), "bitnami.com") {
		indicators = append(indicators, "home field contains bitnami.com")
	}

	// 2. Check sources for GitHub Bitnami references
	for _, source := range metadata.Sources {
		if strings.Contains(strings.ToLower(source), "github.com/bitnami/charts") {
			indicators = append(indicators, "sources reference github.com/bitnami/charts")
		}
	}

	// 3. Check maintainers
	for _, maintainer := range metadata.Maintainers {
		if strings.Contains(strings.ToLower(maintainer.Name), "bitnami") ||
			strings.Contains(strings.ToLower(maintainer.Name), "broadcom") {
			indicators = append(indicators, "maintainer references Bitnami/Broadcom")
		}

		if maintainer.URL != "" &&
			(strings.Contains(strings.ToLower(maintainer.URL), "bitnami") ||
				strings.Contains(strings.ToLower(maintainer.URL), "broadcom")) {
			indicators = append(indicators, "maintainer URL references Bitnami/Broadcom")
		}
	}

	// 4. Check for common Bitnami dependencies
	for _, dep := range ch.Dependencies() {
		if dep != nil && dep.Metadata != nil {
			if strings.Contains(strings.ToLower(dep.Metadata.Name), "bitnami-common") {
				indicators = append(indicators, "dependency references bitnami-common")
			}
		}
	}

	// 5. Check for Bitnami/Broadcom copyright in annotations
	if metadata.Annotations != nil {
		for key, value := range metadata.Annotations {
			if (strings.Contains(strings.ToLower(key), "copyright") ||
				strings.Contains(strings.ToLower(key), "license")) &&
				(strings.Contains(strings.ToLower(value), "bitnami") ||
					strings.Contains(strings.ToLower(value), "broadcom")) {
				indicators = append(indicators, "annotations contain Bitnami/Broadcom copyright")
			}
		}
	}

	// Determine confidence level based on number of indicators
	var confidence ConfidenceLevel
	switch len(indicators) {
	case 0:
		confidence = ConfidenceNone
	case 1:
		confidence = ConfidenceLow
	case bitnamiMediumConfidenceIndicatorCount:
		confidence = ConfidenceMedium
	default:
		confidence = ConfidenceHigh
	}

	// Special case: home field + maintainer is high confidence
	if len(indicators) >= 2 &&
		strings.Contains(strings.Join(indicators, " "), "home field") &&
		strings.Contains(strings.Join(indicators, " "), "maintainer") {
		confidence = ConfidenceHigh
	}

	debug.Printf("Bitnami detection for chart '%s': Confidence=%d, Indicators=%v",
		ch.Name(), confidence, indicators)

	return Detection{
		Provider:   ProviderBitnami,
		Confidence: confidence,
		Indicators: indicators,
	}
}
