package image

import (
	"regexp"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

// isValidRegistryName checks if a string is potentially a valid registry name component.
// Note: This is a basic check. Docker reference spec is complex.
func isValidRegistryName(name string) bool {
	if name == "" {
		return false
	}
	// Basic check: Allow alphanum, dot, dash, colon (for port)
	// Registry component must contain at least one dot, colon, or be "localhost".
	// Relaxed check for now - mainly check for invalid chars like spaces.
	// We need domain name validation basically.
	return !strings.ContainsAny(name, " /\\") // Very basic: no spaces or slashes allowed?
}

// isValidRepositoryName checks if a string is potentially a valid repository name component.
// Allows lowercase alphanum, underscore, dot, dash, and forward slashes for namespaces.
// Must start and end with alphanum. No consecutive separators.
func isValidRepositoryName(repo string) bool {
	if repo == "" {
		return false
	}
	// Check for invalid characters
	pattern := `^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`
	matched, err := regexp.MatchString(pattern, repo)
	if err != nil {
		debug.Printf("Error matching repository pattern: %v", err)
		return false
	}

	// Check for invalid sequences
	if strings.Contains(repo, "//") || strings.Contains(repo, "..") || strings.Contains(repo, "__") ||
		strings.Contains(repo, "--") || strings.Contains(repo, "-_") || strings.Contains(repo, "_-") {
		return false
	}

	return matched
}

// isValidTag checks if a string is a valid tag format.
// Max 128 chars, allowed chars: word characters (alphanum + '_') and '.', '-'. Must not start with '.' or '-'.
func isValidTag(tag string) bool {
	if tag == "" {
		return false
	}
	if len(tag) > 128 {
		return false
	}
	// Check for valid characters
	pattern := `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`
	matched, err := regexp.MatchString(pattern, tag)
	if err != nil {
		debug.Printf("Error matching tag pattern: %v", err)
		return false
	}
	return matched
}

// isValidDigest checks if a string is a valid sha256 digest format.
func isValidDigest(digest string) bool {
	if digest == "" {
		return false
	}
	pattern := `^sha256:[a-fA-F0-9]{64}$`
	matched, err := regexp.MatchString(pattern, digest)
	if err != nil {
		debug.Printf("Error matching digest pattern: %v", err)
		return false
	}
	return matched
}

// isImagePath checks if a given path matches known image patterns and not known non-image patterns.
func isImagePath(path []string) bool {
	pathStr := strings.Join(path, ".")

	// Check against non-image patterns first (more specific overrides)
	for _, re := range nonImagePathRegexps {
		if re.MatchString(pathStr) {
			debug.Printf("Path '%s' matches non-image pattern '%s', returning false.", pathStr, re.String())
			return false
		}
	}

	// Check against image patterns
	for _, re := range imagePathRegexps {
		if re.MatchString(pathStr) {
			debug.Printf("Path '%s' matches image pattern '%s', returning true.", pathStr, re.String())
			return true
		}
	}

	debug.Printf("Path '%s' did not match any known image or non-image patterns, returning false.", pathStr)
	return false
}

// IsValidRepository checks if the repository name conforms to allowed patterns.
func IsValidRepository(repo string) bool {
	// Pattern for valid repository names (based on Docker distribution spec)
	// Allows lowercase alphanumeric characters and separators (., _, -, /)
	const pattern = `^[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*$`
	// errcheck: regexp.MatchString error is always nil, safe to ignore.
	matched, _ := regexp.MatchString(pattern, repo)
	return matched
}

// IsValidTag checks if the image tag conforms to allowed patterns.
// Tags are limited to 128 characters and can contain alphanumeric characters,
// underscores, periods, and hyphens.
func IsValidTag(tag string) bool {
	if tag == "" {
		return false // Tag cannot be empty
	}
	if len(tag) > 128 {
		return false // Tag length exceeds limit
	}
	// Pattern for valid tags: word characters (alphanumeric + underscore) plus period and hyphen.
	// Must start with a word character or number.
	const pattern = `^[a-zA-Z0-9][\w.-]*$`
	// errcheck: regexp.MatchString error is always nil, safe to ignore.
	matched, _ := regexp.MatchString(pattern, tag)
	return matched
}

// IsValidDigest checks if the string is a valid image digest (e.g., sha256:...).
func IsValidDigest(digest string) bool {
	const pattern = `^[a-zA-Z0-9_-]+:[a-fA-F0-9]+$`
	// errcheck: regexp.MatchString error is always nil, safe to ignore.
	matched, _ := regexp.MatchString(pattern, digest)
	return matched
}

func isValidTagOrDigest(tag string) bool {
	// Check length
	if len(tag) > 128 {
		return false
	}

	// Check for valid characters
	pattern := `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`
	matched, err := regexp.MatchString(pattern, tag)
	if err != nil {
		debug.Printf("Error matching tag pattern: %v", err)
		return false
	}
	return matched
}

func isValidDigestOnly(digest string) bool {
	pattern := `^sha256:[a-fA-F0-9]{64}$`
	matched, err := regexp.MatchString(pattern, digest)
	if err != nil {
		debug.Printf("Error matching digest pattern: %v", err)
		return false
	}
	return matched
}
