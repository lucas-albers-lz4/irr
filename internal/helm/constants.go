// Package helm provides internal utilities for interacting with Helm.
package helm

import "github.com/lucas-albers-lz4/irr/pkg/keys"

const (
	// DefaultNamespace is the Kubernetes namespace used when none is specified.
	DefaultNamespace = "default"
	// DefaultChartVersion is a placeholder chart version for mock/test chart metadata.
	DefaultChartVersion = "1.0.0"
)

// ValuesYAML is the default origin filename for chart values.
const ValuesYAML = keys.ValuesYAML
