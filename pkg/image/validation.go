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
	// Regex based on Docker spec (simplified):
	// path-component := [a-z0-9]+(?:(?:[._]|__|[-]*)[a-z0-9]+)*
	// name-component := path-component(?:(?:/path-component)+)?
	// Using a slightly simpler check for now:
	// Allows: a-z, 0-9, '.', '_', '-', '/'
	// Constraints: starts/ends with alphanum, no consecutive separators.
	pattern := `^[a-z0-9]+(?:(?:[._-]|[/])?[a-z0-9]+)*$` // Simplified
	matched, _ := regexp.MatchString(pattern, repo)
	if !matched {
		debug.Printf("Repository '%s' failed regex check '%s'", repo, pattern)
		return false
	}
	// Check for consecutive separators (simplistic)
	if strings.Contains(repo, "//") || strings.Contains(repo, "..") || strings.Contains(repo, "__") || strings.Contains(repo, "--") || strings.Contains(repo, "-_") || strings.Contains(repo, "_-") {
		debug.Printf("Repository '%s' contains consecutive separators.", repo)
		return false
	}
	return true
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
	pattern := `^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`
	matched, _ := regexp.MatchString(pattern, tag)
	return matched
}

// isValidDigest checks if a string is a valid sha256 digest format.
func isValidDigest(digest string) bool {
	if digest == "" {
		return false
	}
	pattern := `^sha256:[a-fA-F0-9]{64}$`
	matched, _ := regexp.MatchString(pattern, digest)
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
