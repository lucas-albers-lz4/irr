//go:build integration

package integration_test

import (
	"testing"
	// Add necessary imports for test execution, assertions, and potentially test chart setup
	// "github.com/stretchr/testify/assert"
	// "github.com/stretchr/testify/require"
	// "github.com/lucas-albers-lz4/irr/pkg/testutil"
)

// TestValidateWithKubeVersionFlag validates that the --kube-version flag works correctly
// against sample charts that require specific Kubernetes versions.
// This test is deferred because it requires a running Helm/Kubernetes environment.
func TestValidateWithKubeVersionFlag(t *testing.T) {
	t.Skip("Deferred: requires a running Helm/Kubernetes environment")
}

// TestValidateKubeVersionOverridesSet validates that --kube-version takes precedence
// over any `--set kubeVersion=...` or `--set Capabilities.KubeVersion.*=...` flags.
// This test is deferred because it requires a running Helm/Kubernetes environment.
func TestValidateKubeVersionOverridesSet(t *testing.T) {
	t.Skip("Deferred: requires a running Helm/Kubernetes environment")
}
