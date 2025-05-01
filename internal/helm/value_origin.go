// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
)

// ValueWithOrigin wraps a value with information about its origin
type ValueWithOrigin struct {
	Value  interface{}
	Origin helmtypes.ValueOrigin
}

// CoalescedValues represents the final merged values with origin information
type CoalescedValues struct {
	// Values contains the final merged values
	Values map[string]interface{}
	// Origins tracks the origin of each value path
	Origins map[string]helmtypes.ValueOrigin
}
