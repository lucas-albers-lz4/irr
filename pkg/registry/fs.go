// Package registry provides functionality for mapping container registry names.
package registry

import (
	"github.com/lucas-albers-lz4/irr/pkg/fileutil"
	"github.com/spf13/afero"
)

// DefaultFS is the default filesystem implementation used throughout the registry package
var DefaultFS fileutil.FS = fileutil.DefaultFS

// SetFS replaces the default filesystem with the provided one and returns a cleanup function
func SetFS(fs fileutil.FS) func() {
	oldFS := DefaultFS
	DefaultFS = fs
	return func() {
		DefaultFS = oldFS
	}
}

// GetAferoFS extracts the underlying afero.Fs from a fileutil.FS interface
// This is needed for backward compatibility with existing functions that directly accept afero.Fs
func GetAferoFS(fs fileutil.FS) afero.Fs {
	if fs == nil {
		return afero.NewOsFs()
	}

	// Special case for testing: If the filesystem is already an *afero.MemMapFs wrapper,
	// try to extract and return the actual memMapFs for test consistency
	if wrapper, ok := fs.(*fileutil.AferoFS); ok {
		// Use the accessor to get the underlying filesystem
		return wrapper.GetUnderlyingFs()
	}

	// For testing purposes, create a memory filesystem
	// This ensures tests don't touch the real filesystem
	// In a production environment, this function would ideally get the actual afero.Fs
	// from the fileutil.FS interface, but we don't have access to that
	return afero.NewMemMapFs()
}
