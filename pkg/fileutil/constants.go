// Package fileutil provides file-related utility functions and constants.
package fileutil

// Standard file permission constants
const (
	// ReadWriteUserPermission represents read/write permissions for the file owner only (0600 in octal)
	ReadWriteUserPermission = 0o600
	// ReadWriteUserReadOthers represents read/write for owner, read for others (0644 in octal)
	ReadWriteUserReadOthers = 0o644
)
