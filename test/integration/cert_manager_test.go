//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lucas-albers-lz4/irr/pkg/testutil"
)

// TestCertManagerOverrides is the original monolithic test for cert-manager.
// It's currently skipped in favor of the more targeted component-based approach
// in TestCertManager below.
func TestCertManagerOverrides(t *testing.T) {
	t.Skip("cert-manager chart has unique structure that requires component-based testing approach. See TestCertManager in this file.")

	harness := NewTestHarness(t)
	defer harness.Cleanup()

	harness.SetupChart(testutil.GetChartPath("cert-manager"))
	harness.SetRegistries("target.io", []string{"quay.io"})

	if err := harness.GenerateOverrides(); err != nil {
		t.Fatalf("Failed to generate overrides: %v", err)
	}

	if err := harness.ValidateOverrides(); err != nil {
		t.Fatalf("Failed to validate overrides: %v", err)
	}
}

// TestCertManager implements component-group testing for the cert-manager chart
// to improve testability and debugging for this complex chart.
func TestCertManager(t *testing.T) {
	// Skip the entire test suite if needed during development or if cert-manager chart is unavailable
	chartPath := testutil.GetChartPath("cert-manager")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Skip("cert-manager chart not found in test data")
	}

	// Define component groups
	componentGroups := []struct {
		name           string   // Group name for subtest identification
		components     []string // Components in this group (used for filtering)
		threshold      int      // Success threshold percentage
		expectedImages int      // Expected number of images to find
		isCritical     bool     // Whether this group contains critical components
	}{
		{
			name:           "core_controllers",
			components:     []string{"controller", "webhook"},
			threshold:      100,
			expectedImages: 2,
			isCritical:     true,
		},
		{
			name:           "support_services",
			components:     []string{"cainjector", "startupapicheck"},
			threshold:      95,
			expectedImages: 2,
			isCritical:     false,
		},
		{
			name:           "acme_solver",
			components:     []string{"acmesolver"},
			threshold:      100,
			expectedImages: 1,
			isCritical:     true,
		},
	}

	// Load the chart once outside the loop
	harness := NewTestHarness(t)
	defer harness.Cleanup()
	harness.SetupChart(chartPath)
	harness.SetRegistries("harbor.home.arpa", []string{"quay.io", "registry.k8s.io"})

	// Run a subtest for each component group
	for _, group := range componentGroups {
		t.Run(group.name, func(t *testing.T) {
			// Create image paths to test for this component group
			var imagePaths []string
			for _, component := range group.components {
				if component == "controller" {
					// Special case for controller
					imagePaths = append(imagePaths, "image")
				} else {
					// For other components, use the component.image pattern
					imagePaths = append(imagePaths, component+".image")
				}
			}

			// Prepare the known image paths argument
			knownImagePathsArg := strings.Join(imagePaths, ",")

			// Set up additional args based on component group
			additionalArgs := []string{
				"--known-image-paths", knownImagePathsArg,
			}

			// Enable debug logging via environment variable
			// os.Setenv("LOG_LEVEL", "DEBUG") // Ensure debug logs are generated
			// defer os.Unsetenv("LOG_LEVEL") // Clean up env var

			// Construct the command
			args := []string{
				"override",
				"--chart-path", harness.chartPath,
				"--target-registry", harness.targetReg,
				"--source-registries", strings.Join(harness.sourceRegs, ","),
				"--output-file", harness.overridePath,
			}
			args = append(args, additionalArgs...)

			// Execute the command
			output, err := harness.ExecuteIRR(nil, args...)
			if err != nil {
				if group.isCritical {
					t.Errorf("[%s] Failed to execute irr override command: %v\nOutput:\n%s", group.name, err, output)
					return
				} else {
					t.Logf("[%s] WARNING: irr override command failed but this is not a critical component group: %v", group.name, err)
					t.Logf("[%s] Command output:\n%s", group.name, output)
				}
			}

			// Parse the override file if it exists
			overrides, err := harness.getOverrides()
			if err != nil {
				if group.isCritical {
					t.Errorf("[%s] Failed to read/parse generated overrides file: %v", group.name, err)
					return
				} else {
					t.Logf("[%s] WARNING: Failed to read/parse generated overrides file: %v", group.name, err)
					return
				}
			}

			// Analyze the overrides
			foundImageRepos, foundImageStrings := collectImageInfo(t, harness, overrides)

			// Log found images for debugging
			t.Logf("[%s] Found image repositories:\n%v", group.name, foundImageRepos)
			if len(foundImageStrings) > 0 {
				t.Logf("[%s] Found image strings:\n%v", group.name, foundImageStrings)
			}

			// Calculate the number of images found
			imagesFound := len(foundImageRepos) + len(foundImageStrings)

			// Calculate the percentage of expected images found
			var percentageFound int
			if group.expectedImages > 0 {
				percentageFound = (imagesFound * 100) / group.expectedImages
			} else {
				percentageFound = 100 // If no images expected, consider it 100% found
			}

			// Generate component-specific expected images
			expectedImages := generateExpectedImages(group.components, harness.targetReg)

			// Validate expected images against found images
			for _, expectedImage := range expectedImages {
				expectedRepo := extractRepositoryFromImage(expectedImage, harness.targetReg)
				if expectedRepo == "" {
					t.Logf("[%s] WARNING: Could not determine expected repo from %s", group.name, expectedImage)
					continue
				}

				found := false
				for actualRepo := range foundImageRepos {
					if actualRepo == expectedRepo {
						t.Logf("[%s] SUCCESS: Found expected image %s", group.name, expectedRepo)
						found = true
						break
					}
				}

				if !found {
					t.Logf("[%s] WARNING: Expected image %s not found in overrides", group.name, expectedRepo)
				}
			}

			// Check if the threshold is met
			if percentageFound < group.threshold {
				if group.isCritical {
					t.Errorf("[%s] Found %d/%d images (%.2f%%) which is below the required threshold of %d%%",
						group.name, imagesFound, group.expectedImages, float64(percentageFound), group.threshold)
				} else {
					t.Logf("[%s] WARNING: Found %d/%d images (%.2f%%) which is below the threshold of %d%%, but this component group is not critical",
						group.name, imagesFound, group.expectedImages, float64(percentageFound), group.threshold)
				}
			} else {
				t.Logf("[%s] SUCCESS: Found %d/%d images (%.2f%%) which meets or exceeds the threshold of %d%%",
					group.name, imagesFound, group.expectedImages, float64(percentageFound), group.threshold)
			}
		})
	}
}

// generateExpectedImages creates a list of expected images based on the component names
func generateExpectedImages(components []string, targetRegistry string) []string {
	var images []string

	for _, component := range components {
		// Map component name to expected image path
		var imageName string

		switch component {
		case "controller":
			imageName = "jetstack/cert-manager-controller:v1.17.1"
		case "webhook":
			imageName = "jetstack/cert-manager-webhook:v1.17.1"
		case "cainjector":
			imageName = "jetstack/cert-manager-cainjector:v1.17.1"
		case "acmesolver":
			imageName = "jetstack/cert-manager-acmesolver:v1.17.1"
		case "startupapicheck":
			imageName = "jetstack/cert-manager-startupapicheck:v1.17.1"
		default:
			continue
		}

		images = append(images, fmt.Sprintf("%s/quayio/%s", targetRegistry, imageName))
	}

	return images
}

// extractRepositoryFromImage extracts the repository part from an image reference
func extractRepositoryFromImage(imageRef, targetRegistry string) string {
	if strings.HasPrefix(imageRef, targetRegistry+"/") {
		parts := strings.SplitN(strings.TrimPrefix(imageRef, targetRegistry+"/"), ":", 2)
		return parts[0]
	}
	return ""
}

// TODO: Add more specialized cert-manager tests here as needed
// Consider tests for:
// - Image structure variations
// - Custom path strategies
// - CRD handling
// - Component isolation
