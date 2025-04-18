package fileutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAbsPath(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		path          string
		expectedError bool
	}{
		{
			name:          "Valid path",
			path:          "utils_test.go",
			expectedError: false,
		},
		{
			name:          "Absolute path",
			path:          "/tmp/test",
			expectedError: false,
		},
		{
			name:          "Empty path",
			path:          "",
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call GetAbsPath
			result, err := GetAbsPath(tc.path)

			// Check error expectation
			if tc.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify the result is an absolute path
			assert.True(t, filepath.IsAbs(result))

			// For the current file, we can verify more precisely
			if tc.path == "utils_test.go" {
				assert.Equal(t, "utils_test.go", filepath.Base(result))
			}
		})
	}

	// Test with a non-existent path that should still resolve
	t.Run("Non-existent path", func(t *testing.T) {
		nonExistentPath := "non_existent_file.txt"
		result, err := GetAbsPath(nonExistentPath)
		require.NoError(t, err)

		// The path should be absolute
		assert.True(t, filepath.IsAbs(result))

		// The base name should match
		assert.Equal(t, nonExistentPath, filepath.Base(result))

		// The file should not exist
		_, err = os.Stat(result)
		assert.True(t, os.IsNotExist(err))
	})
}
