package integration_test

import (
	"testing"
	// Add necessary imports for test execution, assertions, and potentially test chart setup
	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
	// "github.com/lalbers/irr/pkg/testutil"
)

// TestValidateWithKubeVersionFlag validates that the --kube-version flag works correctly
// against sample charts that require specific Kubernetes versions.
func TestValidateWithKubeVersionFlag(t *testing.T) {
	t.Skip("Integration test placeholder: TestValidateWithKubeVersionFlag needs implementation")

	// TODO: Implement integration test
	// 1. Setup: Define or download simple test charts that have specific
	//    `kubeVersion` requirements in their Chart.yaml (e.g., >=1.25, >=1.30).
	// 2. Setup: Prepare necessary dummy override/values files.
	// 3. Execute: Run `irr validate --chart-path <chart> --values <override> --kube-version <version>`
	//    for different combinations:
	//    a) Chart requiring >=1.25, run with --kube-version 1.24.0 (expect failure)
	//    b) Chart requiring >=1.25, run with --kube-version 1.25.0 (expect success)
	//    c) Chart requiring >=1.30, run with --kube-version 1.29.0 (expect failure)
	//    d) Chart requiring >=1.30, run with --kube-version 1.30.0 (expect success)
	//    e) Run without --kube-version (expect default 1.31.0 to succeed for both charts)
	// 4. Assert: Check command exit codes and potentially stderr for expected outcomes.
}

// TestValidateKubeVersionOverridesSet validates that --kube-version takes precedence
// over any `--set kubeVersion=...` or `--set Capabilities.KubeVersion.*=...` flags.
func TestValidateKubeVersionOverridesSet(t *testing.T) {
	t.Skip("Integration test placeholder: TestValidateKubeVersionOverridesSet needs implementation")

	// TODO: Implement integration test
	// 1. Setup: Use a simple test chart (e.g., one requiring >=1.25).
	// 2. Setup: Prepare dummy override/values files.
	// 3. Execute: Run `irr validate` with conflicting flags:
	//    `--kube-version 1.26.0 --set kubeVersion=1.24.0 --set Capabilities.KubeVersion.Version=v1.24.0`
	// 4. Assert: Verify the command succeeds (because 1.26.0 meets the >=1.25 requirement),
	//    indicating that --kube-version took precedence over the --set flags.
	// 5. Execute: Run `irr validate` with conflicting flags where --kube-version would fail:
	//    `--kube-version 1.24.0 --set kubeVersion=1.26.0 --set Capabilities.KubeVersion.Version=v1.26.0`
	// 6. Assert: Verify the command fails, indicating --kube-version took precedence.
}
