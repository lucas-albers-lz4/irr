package image

import (
	// Need this for port stripping
	"net"
	"regexp"
	"strings" // Need this for normalization checks

	"github.com/distribution/reference"
	"github.com/lalbers/irr/pkg/debug"
)

// Constants
const (
	// LatestTag is the default tag used when no tag is specified
	LatestTag = "latest"
)

// Regular expression patterns for image reference parsing
var (
	digestPattern      = regexp.MustCompile(`^(.+)@(.+)$`)
	tagPattern         = regexp.MustCompile(`^(.+):([^/]+)$`)
	repositoryPattern  = regexp.MustCompile(`^(.+)$`)
	registryRepPattern = regexp.MustCompile(`^(.+)/(.+)$`)
	referencePattern   = regexp.MustCompile(`^(.+)/(.+)(@(.+))?(:([^/]+))?$`)
)

// ParseImageReference parses an image reference string into its components.
// It returns a Reference struct or error if the image reference is invalid.
func ParseImageReference(imageRef string) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")

	if imageRef == "" {
		debug.Printf("Empty image reference")
		return nil, ErrEmptyImageReference
	}

	debug.Printf("Parsing image reference: %s", imageRef)

	// Try to parse with distribution/reference library first
	named, err := reference.ParseAnyReference(imageRef)
	if err == nil {
		debug.Printf("Successfully parsed with distribution/reference library")
		result := &Reference{
			Original: imageRef,
		}

		// Extract domain and path
		if namedRef, ok := named.(reference.Named); ok {
			result.Registry = reference.Domain(namedRef)
			result.Repository = reference.Path(namedRef)
		}

		// Extract tag if present
		var hasTag, hasDigest bool
		if taggedRef, ok := named.(reference.Tagged); ok {
			result.Tag = taggedRef.Tag()
			debug.Printf("Extracted tag: %s", result.Tag)
			hasTag = true
		}

		// Extract digest if present
		if digestedRef, ok := named.(reference.Digested); ok {
			result.Digest = digestedRef.Digest().String()
			debug.Printf("Extracted digest: %s", result.Digest)
			hasDigest = true
		}

		// Check if both tag and digest are present (conflicting)
		if hasTag && hasDigest {
			debug.Printf("Warning: Reference has both tag (%s) and digest (%s)", result.Tag, result.Digest)
			return nil, ErrTagAndDigestPresent
		}

		// Strip port from registry domain if present
		if strings.Contains(result.Registry, ":") {
			host, _, err := net.SplitHostPort(result.Registry)
			if err == nil {
				debug.Printf("Stripping port from registry domain: %s → %s", result.Registry, host)
				result.Registry = host
			}
		}

		// Default tag to latest for repository-only references
		if result.Tag == "" && result.Digest == "" {
			result.Tag = LatestTag
			debug.Printf("Setting default tag: %s", result.Tag)
		}

		debug.Printf("Parsed reference: %+v", result)
		return result, nil
	}

	debug.Printf("Distribution/reference library failed to parse: %v. Falling back to regex parsing.", err)

	// Fall back to our regex-based parser for better error messages or special cases
	return parseWithRegex(imageRef)
}

// parseWithRegex parses an image reference using regular expressions.
// This is used as a fallback when the distribution library parser fails.
func parseWithRegex(imageRef string) (*Reference, error) {
	debug.Printf("Using regex parser for: %s", imageRef)

	// Quick validation for common invalid formats
	if strings.Contains(imageRef, "///") || strings.Contains(imageRef, "::") {
		debug.Printf("Invalid image reference format detected: %s", imageRef)
		return nil, ErrInvalidImageReference
	}

	// Check for invalid repository name characters
	invalidChars := []string{" ", "@", "$", "?", "#", "\\"}
	for _, char := range invalidChars {
		if strings.Contains(imageRef, char) {
			debug.Printf("Invalid repository name character detected in: %s", imageRef)
			return nil, ErrInvalidImageReference
		}
	}

	// Check for invalid digest format
	if strings.Contains(imageRef, "@") {
		digestParts := strings.Split(imageRef, "@")
		if len(digestParts) > 1 {
			digest := digestParts[1]
			// Valid digest should be algo:hex where algo is usually sha256
			if !strings.Contains(digest, ":") || !strings.HasPrefix(digest, "sha256:") {
				debug.Printf("Invalid digest format detected: %s", digest)
				return nil, ErrInvalidImageReference
			}
		}
	}

	// Check for invalid tag format
	if strings.Contains(imageRef, ":") && !strings.Contains(imageRef, "@") {
		tagParts := strings.Split(imageRef, ":")
		if len(tagParts) > 1 {
			tag := tagParts[len(tagParts)-1]
			// Quick validation for obviously invalid tag formats
			if strings.Contains(tag, "/") || strings.Contains(tag, "\\") {
				debug.Printf("Invalid tag format detected: %s", tag)
				return nil, ErrInvalidImageReference
			}
		}
	}

	// Special case for repository-only references (no tag or digest)
	if match := repositoryPattern.FindStringSubmatch(imageRef); match != nil && strings.Count(imageRef, "/") == 0 {
		debug.Printf("Matched repository-only pattern")
		return &Reference{
			Original:   imageRef,
			Repository: match[1],
			Tag:        LatestTag,
		}, nil
	}

	// Special case for registry/repository format (no tag or digest)
	if match := registryRepPattern.FindStringSubmatch(imageRef); match != nil && !strings.Contains(imageRef, ":") && !strings.Contains(imageRef, "@") {
		debug.Printf("Matched registry/repository pattern")
		registry := match[1]
		repository := match[2]

		// Strip port from registry domain if present
		if strings.Contains(registry, ":") {
			host, _, err := net.SplitHostPort(registry)
			if err == nil {
				debug.Printf("Stripping port from registry domain: %s → %s", registry, host)
				registry = host
			}
		}

		return &Reference{
			Original:   imageRef,
			Registry:   registry,
			Repository: repository,
			Tag:        LatestTag,
		}, nil
	}

	// Try matching against the comprehensive reference pattern
	if match := referencePattern.FindStringSubmatch(imageRef); match != nil {
		debug.Printf("Matched comprehensive reference pattern")
		result := &Reference{
			Original:   imageRef,
			Registry:   match[1],
			Repository: match[2],
		}

		// Check if both tag and digest are present (conflicting)
		if match[3] != "" && match[5] != "" {
			debug.Printf("Both tag and digest present in reference")
			return nil, ErrTagAndDigestPresent
		}

		// Extract tag if present
		if match[3] != "" {
			result.Tag = match[4]
			debug.Printf("Extracted tag: %s", result.Tag)
		}

		// Extract digest if present
		if match[5] != "" {
			result.Digest = match[6]
			debug.Printf("Extracted digest: %s", result.Digest)
		}

		// Strip port from registry domain if present
		if strings.Contains(result.Registry, ":") {
			host, _, err := net.SplitHostPort(result.Registry)
			if err == nil {
				debug.Printf("Stripping port from registry domain: %s → %s", result.Registry, host)
				result.Registry = host
			}
		}

		// Default tag to latest for repository-only references if neither tag nor digest is present
		if result.Tag == "" && result.Digest == "" {
			result.Tag = LatestTag
			debug.Printf("Setting default tag: %s", result.Tag)
		}

		debug.Printf("Regex parsed reference: %+v", result)
		return result, nil
	}

	debug.Printf("Failed to match reference against any pattern")
	return nil, ErrInvalidImageReference
}

// isValidRepositoryName validates repository name format
func isValidRepositoryName(name string) bool {
	// Simple validation - repository name must not be empty
	// and should not contain invalid characters
	if name == "" {
		return false
	}

	// Check for invalid characters (simplified validation)
	invalidChars := []string{" ", "\\", "$", "?", "#"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return false
		}
	}

	return true
}

// isValidTag validates tag format
func isValidTag(tag string) bool {
	// Simple validation - tag must not be empty and should match the tag pattern
	if tag == "" {
		return false
	}

	// Tag should only contain alphanumeric characters, periods, dashes, and underscores
	validTagPattern := regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)
	return validTagPattern.MatchString(tag)
}

// // parseImageReferenceCustom is deprecated. // REMOVED UNUSED
// func parseImageReferenceCustom(imageStr string) (Reference, error) { // REMOVED UNUSED
// 	return Reference{}, errors.New("parseImageReferenceCustom is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED
// // parseRegistryRepo is deprecated. // REMOVED UNUSED
// func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) { // REMOVED UNUSED
// 	return "", "", errors.New("parseRegistryRepo is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED
