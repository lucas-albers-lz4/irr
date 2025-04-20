package rules

import (
	"testing"

	"helm.sh/helm/v3/pkg/chart"
)

func TestDetectBitnamiChart(t *testing.T) {

	// Helper function for assertion logic
	assertDetection := func(t *testing.T, got, want Detection) {
		t.Helper()
		// Check provider
		if got.Provider != want.Provider {
			t.Errorf("provider = %v, want %v", got.Provider, want.Provider)
		}

		// Check confidence level
		if got.Confidence != want.Confidence {
			t.Errorf("confidence = %v, want %v", got.Confidence, want.Confidence)
		}

		// Check indicators (just the count, as the exact order might vary)
		// TODO: Consider comparing indicator content more precisely if needed
		if len(got.Indicators) != len(want.Indicators) {
			t.Errorf("indicators count = %v, want %v (Got: %v)",
				len(got.Indicators), len(want.Indicators), got.Indicators)
		}
	}

	t.Run("High confidence - multiple indicators", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "test-chart",
			Home: "https://bitnami.com/chart",
			Sources: []string{
				"https://github.com/bitnami/charts/tree/main/test",
			},
			Maintainers: []*chart.Maintainer{
				{
					Name: "Bitnami Team",
					URL:  "https://github.com/bitnami",
				},
			},
		}
		expected := Detection{
			Provider:   ProviderBitnami,
			Confidence: ConfidenceHigh,
			Indicators: []string{
				"home field contains bitnami.com",
				"sources reference github.com/bitnami/charts",
				"maintainer references Bitnami/Broadcom",
				"maintainer URL references Bitnami/Broadcom",
			},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})

	t.Run("Low confidence - home field only", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "test-chart",
			Home: "https://bitnami.com/chart",
		}
		expected := Detection{
			Provider:   ProviderBitnami,
			Confidence: ConfidenceLow,
			Indicators: []string{
				"home field contains bitnami.com",
			},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})

	t.Run("No confidence - no indicators", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "test-chart",
			Home: "https://example.com/chart",
		}
		expected := Detection{
			Provider:   ProviderBitnami, // Note: Provider is still Bitnami as detection fn assumes this
			Confidence: ConfidenceNone,
			Indicators: []string{},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})

	t.Run("Low confidence - copyright in annotations", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "test-chart",
			Annotations: map[string]string{
				"licenses":  "Apache-2.0",
				"copyright": "Copyright Broadcom, Inc. All Rights Reserved.",
			},
			Home: "https://charts.example.com", // No other bitnami indicators
		}
		expected := Detection{
			Provider:   ProviderBitnami,
			Confidence: ConfidenceLow,
			Indicators: []string{
				"annotations contain Bitnami/Broadcom copyright",
			},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})

	t.Run("Medium confidence - common dependency via tag only and home field", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "dep-tag-only",
			Dependencies: []*chart.Dependency{
				{
					Name: "common", // Doesn't match "bitnami-common"
					Tags: []string{"bitnami-common"},
				},
			},
			Home: "https://bitnami.com/chart", // Second indicator
		}
		expected := Detection{
			Provider:   ProviderBitnami,
			Confidence: ConfidenceMedium,
			Indicators: []string{"home field contains bitnami.com", "dependency references bitnami-common"},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})

	t.Run("Low confidence - common dependency via name only", func(t *testing.T) {
		metadata := &chart.Metadata{
			Name: "test-chart",
			Dependencies: []*chart.Dependency{
				{
					Name: "bitnami-common", // Name matches
				},
			},
			Home: "https://other.com", // No other indicators
		}
		expected := Detection{
			Provider:   ProviderBitnami,
			Confidence: ConfidenceLow,
			Indicators: []string{
				"dependency references bitnami-common",
			},
		}
		ch := &chart.Chart{Metadata: metadata, Lock: &chart.Lock{}}
		got := detectBitnamiChart(ch)
		assertDetection(t, got, expected)
	})
}

func TestAppliesTo(t *testing.T) {
	// Create a test chart that should match Bitnami detection
	bitnamiChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "test-bitnami-chart",
			Home: "https://bitnami.com/charts",
			Maintainers: []*chart.Maintainer{
				{
					Name: "Bitnami Team",
				},
			},
		},
	}

	// Create a test chart that should NOT match Bitnami detection
	nonBitnamiChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name: "test-standard-chart",
			Home: "https://example.com/charts",
		},
	}

	// Create the rule to test
	rule := NewBitnamiSecurityBypassRule()

	// Test the Bitnami chart
	detection, applies := rule.AppliesTo(bitnamiChart)
	if !applies {
		t.Errorf("BitnamiSecurityBypassRule.AppliesTo() should apply to Bitnami chart")
	}
	if detection.Provider != ProviderBitnami || detection.Confidence < ConfidenceMedium {
		t.Errorf("BitnamiSecurityBypassRule.AppliesTo() detection = %v, expected Bitnami with confidence >= Medium", detection)
	}

	// Test the non-Bitnami chart
	_, applies = rule.AppliesTo(nonBitnamiChart)
	if applies {
		t.Errorf("BitnamiSecurityBypassRule.AppliesTo() should not apply to non-Bitnami chart")
	}
}

func TestDetectChartProvider(t *testing.T) {
	tests := []struct {
		name               string
		chart              *chart.Chart
		expectedProvider   ChartProviderType
		expectedConfidence ConfidenceLevel
	}{
		{
			name: "Bitnami chart with high confidence",
			chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name: "high-confidence-bitnami",
					Home: "https://bitnami.com/chart",
					Sources: []string{
						"https://github.com/bitnami/charts/tree/main/test",
					},
					Maintainers: []*chart.Maintainer{
						{
							Name: "Bitnami Team",
							URL:  "https://github.com/bitnami",
						},
					},
				},
			},
			expectedProvider:   ProviderBitnami,
			expectedConfidence: ConfidenceHigh,
		},
		{
			name: "Bitnami chart with medium confidence",
			chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name: "medium-confidence-bitnami",
					Home: "https://bitnami.com/chart",
				},
			},
			expectedProvider:   ProviderBitnami,
			expectedConfidence: ConfidenceLow, // Single indicator is low confidence
		},
		{
			name: "Unknown provider chart",
			chart: &chart.Chart{
				Metadata: &chart.Metadata{
					Name: "unknown-provider",
					Home: "https://example.com/chart",
				},
			},
			expectedProvider:   ProviderUnknown,
			expectedConfidence: ConfidenceNone,
		},
		{
			name:               "Nil chart",
			chart:              nil,
			expectedProvider:   ProviderUnknown,
			expectedConfidence: ConfidenceNone,
		},
		{
			name: "Chart with nil metadata",
			chart: &chart.Chart{
				Metadata: nil,
			},
			expectedProvider:   ProviderUnknown,
			expectedConfidence: ConfidenceNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run detection
			detection := DetectChartProvider(tt.chart)

			// Check provider
			if detection.Provider != tt.expectedProvider {
				t.Errorf("DetectChartProvider() provider = %v, want %v", detection.Provider, tt.expectedProvider)
			}

			// Check confidence level
			if detection.Confidence != tt.expectedConfidence {
				t.Errorf("DetectChartProvider() confidence = %v, want %v", detection.Confidence, tt.expectedConfidence)
			}
		})
	}
}
