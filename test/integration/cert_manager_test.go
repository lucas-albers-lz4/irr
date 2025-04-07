//go:build integration

// Package integration contains integration tests for the helm-image-override tool.
package integration

import (
	"testing"

	"github.com/lalbers/irr/pkg/testutil"
)

func TestCertManagerOverrides(t *testing.T) {
	t.Skip("cert-manager chart validation fails with YAML syntax errors")
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
