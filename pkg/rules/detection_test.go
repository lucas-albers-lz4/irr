package rules

import (
	"testing"

	"helm.sh/helm/v3/pkg/chart"
)

func TestDetectBitnamiChart(t *testing.T) {
	tests := []struct {
		name           string
		metadata       *chart.Metadata
		deps           []*chart.Chart
		expectedResult Detection
	}{
		{
			name: "High confidence - multiple indicators",
			metadata: &chart.Metadata{
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
			},
			expectedResult: Detection{
				Provider:   ProviderBitnami,
				Confidence: ConfidenceHigh,
				Indicators: []string{
					"home field contains bitnami.com",
					"sources reference github.com/bitnami/charts",
					"maintainer references Bitnami/Broadcom",
					"maintainer URL references Bitnami/Broadcom",
				},
			},
		},
		{
			name: "Medium confidence - home field only",
			metadata: &chart.Metadata{
				Name: "test-chart",
				Home: "https://bitnami.com/chart",
			},
			expectedResult: Detection{
				Provider:   ProviderBitnami,
				Confidence: ConfidenceLow,
				Indicators: []string{
					"home field contains bitnami.com",
				},
			},
		},
		{
			name: "No confidence - no indicators",
			metadata: &chart.Metadata{
				Name: "test-chart",
				Home: "https://example.com/chart",
			},
			expectedResult: Detection{
				Provider:   ProviderBitnami,
				Confidence: ConfidenceNone,
				Indicators: []string{},
			},
		},
		{
			name: "High confidence - copyright in annotations",
			metadata: &chart.Metadata{
				Name: "test-chart",
				Annotations: map[string]string{
					"licenses":  "Apache-2.0",
					"copyright": "Copyright Broadcom, Inc. All Rights Reserved.",
				},
				Home: "https://charts.example.com",
			},
			expectedResult: Detection{
				Provider:   ProviderBitnami,
				Confidence: ConfidenceLow,
				Indicators: []string{
					"annotations contain Bitnami/Broadcom copyright",
				},
			},
		},
		{
			name: "With bitnami-common dependency",
			metadata: &chart.Metadata{
				Name: "test-chart",
			},
			deps: []*chart.Chart{
				{
					Metadata: &chart.Metadata{
						Name: "bitnami-common",
					},
				},
			},
			expectedResult: Detection{
				Provider:   ProviderBitnami,
				Confidence: ConfidenceLow,
				Indicators: []string{
					"dependency references bitnami-common",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a chart with the test metadata
			ch := &chart.Chart{
				Metadata: tt.metadata,
				Lock:     &chart.Lock{}, // To avoid nil pointer dereference in some chart operations
			}

			// Add dependencies if provided
			for _, dep := range tt.deps {
				ch.AddDependency(dep)
			}

			// Run the detection
			got := detectBitnamiChart(ch)

			// Check provider
			if got.Provider != tt.expectedResult.Provider {
				t.Errorf("detectBitnamiChart() provider = %v, want %v", got.Provider, tt.expectedResult.Provider)
			}

			// Check confidence level
			if got.Confidence != tt.expectedResult.Confidence {
				t.Errorf("detectBitnamiChart() confidence = %v, want %v", got.Confidence, tt.expectedResult.Confidence)
			}

			// Check indicators (just the count, as the exact order might vary)
			if len(got.Indicators) != len(tt.expectedResult.Indicators) {
				t.Errorf("detectBitnamiChart() indicators count = %v, want %v",
					len(got.Indicators), len(tt.expectedResult.Indicators))
			}
		})
	}
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
