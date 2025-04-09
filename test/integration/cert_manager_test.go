//go:build integration

// Package integration contains integration tests for the irr CLI tool.
package integration

import (
	"testing"

	"github.com/lalbers/irr/pkg/testutil"
)

// TestCertManagerOverrides is the original monolithic test for cert-manager.
// It's currently skipped in favor of the more targeted component-based approach
// in TestCertManagerComponents (in integration_test.go).
func TestCertManagerOverrides(t *testing.T) {
	t.Skip("cert-manager chart has unique structure that requires component-based testing approach. See TestCertManagerComponents in integration_test.go")

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

// TODO: Add more specialized cert-manager tests here as needed
// Consider tests for:
// - Image structure variations
// - Custom path strategies
// - CRD handling
// - Component isolation
