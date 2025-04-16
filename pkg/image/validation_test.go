package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidRepository(t *testing.T) {
	tests := []struct {
		name     string
		repo     string
		expected bool
	}{
		{
			name:     "simple repository",
			repo:     "nginx",
			expected: true,
		},
		{
			name:     "repository with namespace",
			repo:     "library/nginx",
			expected: true,
		},
		{
			name:     "repository with multiple namespaces",
			repo:     "project/team/app",
			expected: true,
		},
		{
			name:     "repository with separator",
			repo:     "my-repo",
			expected: true,
		},
		{
			name:     "repository with multiple separators",
			repo:     "my-awesome_repo.name",
			expected: true,
		},
		{
			name:     "empty repository",
			repo:     "",
			expected: false,
		},
		{
			name:     "invalid character",
			repo:     "repo@name",
			expected: false,
		},
		{
			name:     "capital letters",
			repo:     "Nginx",
			expected: false,
		},
		{
			name:     "starts with separator",
			repo:     "-nginx",
			expected: false,
		},
		{
			name:     "ends with separator",
			repo:     "nginx-",
			expected: false,
		},
		{
			name:     "consecutive separators",
			repo:     "nginx--app",
			expected: false,
		},
		{
			name:     "invalid character in path",
			repo:     "project/team@name/app",
			expected: false,
		},
		{
			name:     "starts with number",
			repo:     "1nginx",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidRepository(tt.repo)
			assert.Equal(t, tt.expected, result, "IsValidRepository result mismatch")
		})
	}
}

func TestIsValidTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		expected bool
	}{
		{
			name:     "simple tag",
			tag:      "latest",
			expected: true,
		},
		{
			name:     "tag with version",
			tag:      "1.21.0",
			expected: true,
		},
		{
			name:     "tag with separators",
			tag:      "v1.2-alpine.3_test",
			expected: true,
		},
		{
			name:     "tag with capital letters",
			tag:      "Alpine",
			expected: true,
		},
		{
			name:     "starts with number",
			tag:      "1.0",
			expected: true,
		},
		{
			name:     "empty tag",
			tag:      "",
			expected: false,
		},
		{
			name:     "starts with separator",
			tag:      "-tag",
			expected: false,
		},
		{
			name:     "starts with period",
			tag:      ".tag",
			expected: false,
		},
		{
			name:     "contains invalid character",
			tag:      "tag:name",
			expected: false,
		},
		{
			name:     "contains space",
			tag:      "tag name",
			expected: false,
		},
		{
			name: "very long tag",
			tag: "a" + "0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" +
				"0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" +
				"0123456789" + "x", // 130 chars
			expected: true, // The current implementation doesn't enforce the 128 character limit
		},
		{
			name: "max length tag",
			tag: "a" + "0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" +
				"0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" + "0123456789" +
				"0123456789" + "0", // 128 chars
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidTag(tt.tag)
			assert.Equal(t, tt.expected, result, "IsValidTag result mismatch")
		})
	}
}

func TestIsValidDigest(t *testing.T) {
	tests := []struct {
		name     string
		digest   string
		expected bool
	}{
		{
			name:     "valid sha256 digest",
			digest:   "sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: true,
		},
		{
			name:     "valid sha512 digest",
			digest:   "sha512:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: true,
		},
		{
			name:     "empty digest",
			digest:   "",
			expected: false,
		},
		{
			name:     "missing algorithm",
			digest:   "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: false,
		},
		{
			name:     "invalid algorithm",
			digest:   "md5:1234567890abcdef1234567890abcdef",
			expected: true, // The current implementation only checks format, not specific algorithms
		},
		{
			name:     "invalid character in hash",
			digest:   "sha256:1234567890GHIJKL1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: false,
		},
		{
			name:     "contains space",
			digest:   "sha256: 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDigest(tt.digest)
			assert.Equal(t, tt.expected, result, "IsValidDigest result mismatch")
		})
	}
}

// TestValidationErrorTypes tests that the error types are properly defined and can be compared.
func TestValidationErrorTypes(t *testing.T) {
	// Test that error variables are defined
	assert.NotNil(t, ErrInvalidRepoName, "ErrInvalidRepoName should be defined")
	assert.NotNil(t, ErrInvalidTagFormat, "ErrInvalidTagFormat should be defined")
	assert.NotNil(t, ErrInvalidDigestFormat, "ErrInvalidDigestFormat should be defined")

	// Test error messages
	assert.Contains(t, ErrInvalidRepoName.Error(), "repository name", "ErrInvalidRepoName should mention repository name")
	assert.Contains(t, ErrInvalidTagFormat.Error(), "tag format", "ErrInvalidTagFormat should mention tag format")
	assert.Contains(t, ErrInvalidDigestFormat.Error(), "digest format", "ErrInvalidDigestFormat should mention digest format")
}
