package image

import (
	// Need this for port stripping
	"net"
	"regexp"
	"strings" // Need this for normalization checks

	"github.com/distribution/reference"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

// Constants
const (
	// DefaultTag is the default tag used when no tag is specified
	DefaultTag = "latest"
	// DefaultRegistry is the default registry used when no registry is specified
	DefaultRegistry = "docker.io"
	// LegacyDefaultRegistry is the legacy default registry domain
	LegacyDefaultRegistry = "index.docker.io"
	// OfficialRepositoryName is the repository name for official Docker images
	OfficialRepositoryName = "library"
	// DefaultSeparator is the default separator used in image names
	DefaultSeparator = "/"
	// TagSeparator is the character used to separate the name and tag
	TagSeparator = ":"
	// DigestSeparator is the character used to separate the name and digest
	DigestSeparator = "@"
	// MaxLength is the maximum length of a Docker image reference
	MaxLength = 255
	// MaxTagLength is the maximum length of a tag
	MaxTagLength = 128
	// LatestTag represents the latest tag
	LatestTag = "latest"
)

// Constants for validation (Currently unused, kept for potential future reference)
/*
const (
	alphaNumericRegexString = `[a-z0-9]+`
	separatorRegexString    = `(?:[._]|__|[-]+)`
	domainComponentRegexString = `(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])`
	domainRegexString       = domainComponentRegexString + `(?:\.` + domainComponentRegexString + `)*` + `(?:` + portRegexString + `)?`
	portRegexString         = `:[0-9]+`
	nameComponentRegexString = alphaNumericRegexString + `(?:` + separatorRegexString + alphaNumericRegexString + `)*`
)
*/

// Regular expression patterns for image reference parsing
var (
	// Removed unused patterns: digestPattern, tagPattern
	// Removed unused regex constants: alphaNumericRegexString, separatorRegexString, domainComponentRegexString, domainRegexString, portRegexString, nameComponentRegexString
	// Removed unused var: nameRegex

	// repositoryPattern extracts the repository part of an image reference.
	repositoryPattern = regexp.MustCompile(`^(.+)$`)
	// registryRepPattern extracts the registry and repository parts.
	registryRepPattern = regexp.MustCompile(`^(.+)/(.+)$`)
	// referencePattern is the comprehensive pattern for image references
	referencePattern = regexp.MustCompile(`^(.+)/(.+)(@(.+))?(:([^/]+))?$`)
)

// ParseImageReference parses an image reference string into its components.
// It attempts to parse using the distribution/reference library first, and falls back
// to regex-based parsing if needed for better error messages or special cases.
// The fallback is particularly useful for providing more specific error types
// (like ErrTagAndDigestPresent) or handling formats not strictly covered by the library.
//
// The function handles various image reference formats:
// - registry/repository:tag (e.g., docker.io/nginx:1.23)
// - repository:tag (e.g., nginx:1.23, implies docker.io registry)
// - registry/repository@digest (e.g., docker.io/nginx@sha256:abc...)
// - repository@digest (e.g., nginx@sha256:abc...)
//
// For single-name images like "nginx", it defaults to the "latest" tag.
//
// Parameters:
//   - imageRef: The image reference string to parse
//
// Returns:
//   - *Reference: A Reference struct containing the parsed components
//   - error: An error if the image reference is invalid or cannot be parsed
func ParseImageReference(imageRef string) (*Reference, error) {
	log.Debug("Enter: ParseImageReference")
	defer log.Debug("Exit: ParseImageReference")

	if imageRef == "" {
		log.Debug("Empty image reference")
		return nil, ErrEmptyImageReference
	}

	log.Debug("Parsing image reference: %s", imageRef)

	// Try to parse with distribution/reference library first
	named, err := reference.ParseAnyReference(imageRef)
	if err == nil {
		log.Debug("Successfully parsed with distribution/reference library")
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
			log.Debug("Extracted tag: %s", result.Tag)
			hasTag = true
		}

		// Extract digest if present
		if digestedRef, ok := named.(reference.Digested); ok {
			result.Digest = digestedRef.Digest().String()
			log.Debug("Extracted digest: %s", result.Digest)
			hasDigest = true
		}

		// Check if both tag and digest are present (conflicting)
		if hasTag && hasDigest {
			log.Debug("Warning: Reference has both tag (%s) and digest (%s)", result.Tag, result.Digest)
			return nil, ErrTagAndDigestPresent
		}

		// Strip port from registry domain if present
		if strings.Contains(result.Registry, ":") {
			host, _, err := net.SplitHostPort(result.Registry)
			if err == nil {
				log.Debug("Stripping port from registry domain: %s → %s", result.Registry, host)
				result.Registry = host
			}
		}

		// Default tag to latest for repository-only references
		if result.Tag == "" && result.Digest == "" {
			result.Tag = LatestTag
			log.Debug("Setting default tag: %s", result.Tag)
		}

		log.Debug("Parsed reference: %+v", result)
		return result, nil
	}

	log.Debug("Distribution/reference library failed to parse: %v. Falling back to regex parsing.", err)

	// Fall back to our regex-based parser for better error messages or special cases
	return parseWithRegex(imageRef)
}

// parseWithRegex parses an image reference using regular expressions.
// This is used as a fallback when the distribution library parser fails.
func parseWithRegex(imageRef string) (*Reference, error) {
	log.Debug("Using regex parser for: %s", imageRef)

	// Quick validation for common invalid formats
	if strings.Contains(imageRef, "///") || strings.Contains(imageRef, "::") {
		log.Debug("Invalid image reference format detected: %s", imageRef)
		return nil, ErrInvalidImageReference
	}

	// Check for invalid repository name characters
	invalidChars := []string{" ", "@", "$", "?", "#", "\\"}
	for _, char := range invalidChars {
		if strings.Contains(imageRef, char) {
			log.Debug("Invalid repository name character detected in: %s", imageRef)
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
				log.Debug("Invalid digest format detected: %s", digest)
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
				log.Debug("Invalid tag format detected: %s", tag)
				return nil, ErrInvalidImageReference
			}
		}
	}

	// Special case for repository-only references (no tag or digest)
	if match := repositoryPattern.FindStringSubmatch(imageRef); match != nil && strings.Count(imageRef, "/") == 0 {
		log.Debug("Matched repository-only pattern")
		return &Reference{
			Original:   imageRef,
			Repository: match[1],
			Tag:        LatestTag,
		}, nil
	}

	// Special case for registry/repository format (no tag or digest)
	if match := registryRepPattern.FindStringSubmatch(imageRef); match != nil && !strings.Contains(imageRef, ":") && !strings.Contains(imageRef, "@") {
		log.Debug("Matched registry/repository pattern")
		registry := match[1]
		repository := match[2]

		// Strip port from registry domain if present
		if strings.Contains(registry, ":") {
			host, _, err := net.SplitHostPort(registry)
			if err == nil {
				log.Debug("Stripping port from registry domain: %s → %s", registry, host)
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
		log.Debug("Matched comprehensive reference pattern")
		result := &Reference{
			Original:   imageRef,
			Registry:   match[1],
			Repository: match[2],
		}

		// Check if both tag and digest are present (conflicting)
		if match[3] != "" && match[5] != "" {
			log.Debug("Both tag and digest present in reference")
			return nil, ErrTagAndDigestPresent
		}

		// Extract tag if present
		if match[3] != "" {
			result.Tag = match[4]
			log.Debug("Extracted tag: %s", result.Tag)
		}

		// Extract digest if present
		if match[5] != "" {
			result.Digest = match[6]
			log.Debug("Extracted digest: %s", result.Digest)
		}

		// Strip port from registry domain if present
		if strings.Contains(result.Registry, ":") {
			host, _, err := net.SplitHostPort(result.Registry)
			if err == nil {
				log.Debug("Stripping port from registry domain: %s → %s", result.Registry, host)
				result.Registry = host
			}
		}

		// Default tag to latest for repository-only references if neither tag nor digest is present
		if result.Tag == "" && result.Digest == "" {
			result.Tag = LatestTag
			log.Debug("Setting default tag: %s", result.Tag)
		}

		log.Debug("Regex parsed reference: %+v", result)
		return result, nil
	}

	log.Debug("Failed to match reference against any pattern")
	return nil, ErrInvalidImageReference
}

// // parseImageReferenceCustom is deprecated. // REMOVED UNUSED
// func parseImageReferenceCustom(imageStr string) (Reference, error) { // REMOVED UNUSED
// 	return Reference{}, errors.New("parseImageReferenceCustom is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED
// // parseRegistryRepo is deprecated. // REMOVED UNUSED
// func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) { // REMOVED UNUSED
// 	return "", "", errors.New("parseRegistryRepo is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED

// ---
// Logging migration progress note:
// - pkg/image/parser.go: All debug logging migrated to slog-based logger (log.Debug, log.Error, log.Warn).
// - All debug.* calls replaced with slog style logging.
// - Next: Continue migration in other files using the debug package.
// ---
