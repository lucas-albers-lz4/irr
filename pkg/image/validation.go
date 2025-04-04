package image

import (
	"regexp"
	"strings"
)

// isValidTag checks if a tag string is valid
func isValidTag(tag string) bool {
	// Tags should not be empty
	if tag == "" {
		return false
	}

	// Tags should not contain certain characters
	invalidChars := `!@#$%^&*()+={}[]|\;'"<>?,`
	for _, char := range invalidChars {
		if strings.ContainsRune(tag, char) {
			return false
		}
	}

	return true
}

// isValidImageReference performs additional validation on an image reference.
func isValidImageReference(ref *ImageReference) bool {
	// Repository should not be empty
	if ref.Repository == "" {
		return false
	}

	// Repository should not contain invalid characters
	invalidChars := `!@#$%^&*()+={}[]|\;'"<>?,`
	for _, char := range invalidChars {
		if strings.ContainsRune(ref.Repository, char) {
			return false
		}
	}

	// Tag or digest should look valid if present
	if ref.Tag != "" {
		// Tags typically don't contain certain characters
		invalidTagChars := `!@#$%^&*()+={}[]|\;'"<>?,`
		for _, char := range invalidTagChars {
			if strings.ContainsRune(ref.Tag, char) {
				return false
			}
		}
	}

	if ref.Digest != "" {
		// Digests should be in sha256:... format
		if !strings.HasPrefix(ref.Digest, "sha256:") {
			return false
		}
		// Digest should be 64 characters after sha256:
		digestParts := strings.SplitN(ref.Digest, ":", 2)
		if len(digestParts) != 2 || len(digestParts[1]) != 64 {
			return false
		}
		// Digest should only contain hexadecimal characters
		for _, c := range digestParts[1] {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}

	return true
}

// isValidDockerLibraryName checks if a name is valid for Docker Library
func isValidDockerLibraryName(name string) bool {
	// Docker Library names are typically well-known base images
	commonBaseImages := []string{
		"alpine", "ubuntu", "debian", "centos", "fedora",
		"nginx", "httpd", "redis", "mysql", "postgres",
		"mongo", "node", "python", "ruby", "php",
		"openjdk", "golang", "busybox", "scratch",
	}

	for _, base := range commonBaseImages {
		if name == base {
			return true
		}
	}

	// Also check if it follows Docker Library naming rules
	match, _ := regexp.MatchString("^[a-z0-9]+(?:[._-][a-z0-9]+)*$", name)
	return match
}

// isValidRepositoryPart checks if a repository path part is valid
func isValidRepositoryPart(part string) bool {
	// Repository parts must be lowercase and can contain only
	// alphanumeric characters, dots, hyphens, and underscores
	match, _ := regexp.MatchString("^[a-z0-9]+(?:[._-][a-z0-9]+)*$", part)
	return match
}
