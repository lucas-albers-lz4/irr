// Package fileutil provides file-related utility functions and constants.
package fileutil

// Standard file permission constants
const (
	// ReadWriteUserPermission represents read/write permissions for the file owner only (0600 in octal)
	ReadWriteUserPermission = 0o600
	// ReadWriteUserReadOthers represents read/write for owner, read for group/others (0644 in octal) - Common default for files.
	ReadWriteUserReadOthers = 0o644
	// ReadWriteExecuteUserReadExecuteOthers represents read/write/execute for owner, read/execute for group/others (0755 in octal) - Common default for directories.
	ReadWriteExecuteUserReadExecuteOthers = 0o755
	// ReadWriteExecuteUserReadGroup represents read/write/execute for owner, read/execute for group (0750 in octal) - More restrictive directory permission.
	ReadWriteExecuteUserReadGroup = 0o750
)
