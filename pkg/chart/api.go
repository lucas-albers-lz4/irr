// Package chart provides functionality for loading and processing Helm charts.
package chart

import (
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
)

// NewLoader creates a new DefaultLoader instance with the default filesystem.
// This is a convenience function for clients that don't need to customize the filesystem.
func NewLoader() Loader {
	return NewDefaultLoader(fileutil.DefaultFS)
}
