package image

import (
	// Need this for port stripping

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
// For single-name images like "nginx", it defaults to the "latest" tag unless
// chartMetadata is provided with a non-empty AppVersion value.
// If chartMetadata is provided with AppVersion, that value is used as the tag
// instead of "latest".
func ParseImageReference(imageRef string, chartMetadata ...*ChartMetadata) (*Reference, error) {
	log.Debug("Enter: ParseImageReference")
	log.Debug("Parsing image reference: %s", imageRef)

	// Try to parse with the distribution/reference library
	ref, err := reference.ParseNormalizedNamed(imageRef)
	if err == nil {
		// Successful parse with the canonical library
		registry := reference.Domain(ref)
		repository := reference.Path(ref)
		tag := ""
		digest := ""

		// Check for tag and digest
		if tagged, ok := ref.(reference.Tagged); ok {
			tag = tagged.Tag()
		}
		if digested, ok := ref.(reference.Digested); ok {
			digest = digested.Digest().String()
		}

		// If no tag but we have chartMetadata with AppVersion, use that
		if tag == "" && len(chartMetadata) > 0 && chartMetadata[0] != nil && chartMetadata[0].AppVersion != "" {
			tag = chartMetadata[0].AppVersion
			log.Debug("Using Chart.AppVersion for tag: %s", tag)
		} else if tag == "" && digest == "" {
			// Default to latest tag only if no tag and no digest
			tag = LatestTag
			log.Debug("Setting default tag: %s", tag)
		}

		parsedRef := &Reference{
			Original:   imageRef,
			Registry:   registry,
			Repository: repository,
			Tag:        tag,
			Digest:     digest,
			Detected:   false,
		}
		log.Debug("Parsed reference: %+v", parsedRef)
		log.Debug("Exit: ParseImageReference")
		return parsedRef, nil
	}

	// Fallback to regex-based parsing for better error messages
	// or to handle edge cases not covered by the canonical library
	log.Debug("Falling back to regex parsing after canonical parser error: %v", err)
	return parseWithRegex(imageRef, chartMetadata...)
}

// parseWithRegex parses an image reference using regular expressions.
// This is used as a fallback when the distribution library parser fails.
func parseWithRegex(imageRef string, chartMetadata ...*ChartMetadata) (*Reference, error) {
	log.Debug("Using regex parser for: %s", imageRef)

	// Quick validation for common invalid formats
	if strings.Contains(imageRef, "///") || strings.Contains(imageRef, "::") {
		log.Debug("Invalid image reference format detected: %s", imageRef)
		return nil, ErrInvalidImageReference
	}

	// Check for invalid repository name characters
	invalidChars := []string{" ", "@", "$", "?", "#", "\\"}
	for _, char := range invalidChars {
		if strings.Contains(imageRef, char) && !strings.Contains(imageRef, "@sha256:") {
			log.Debug("Invalid repository name character detected in: %s", imageRef)
			return nil, ErrInvalidImageReference
		}
	}

	// Initialize reference with defaults
	ref := &Reference{
		Original: imageRef,
		Registry: DefaultRegistry, // Default registry (docker.io)
	}

	// Check for both tag and digest - this is invalid
	if strings.Contains(imageRef, ":") && strings.Contains(imageRef, "@") {
		log.Debug("Both tag and digest found in: %s", imageRef)
		return nil, ErrTagAndDigestPresent
	}

	// Handle digest format
	if strings.Contains(imageRef, "@") {
		parts := strings.SplitN(imageRef, "@", 2)
		repoPath := parts[0]
		ref.Digest = parts[1]

		// Extract registry/repository from the part before '@'
		if strings.Contains(repoPath, "/") {
			// Check for possible registry prefix
			pathParts := strings.SplitN(repoPath, "/", 2)
			if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") || pathParts[0] == "localhost" {
				// This looks like a registry
				ref.Registry = pathParts[0]
				ref.Repository = pathParts[1]
			} else {
				// No registry, just a multi-part repository
				ref.Repository = repoPath
			}
		} else {
			// No registry specified and single-part repository
			ref.Repository = repoPath
		}

		// Apply Docker Hub library/ prefix for single name repositories
		if ref.Registry == DefaultRegistry && !strings.Contains(ref.Repository, "/") {
			ref.Repository = "library/" + ref.Repository
		}

		return ref, nil
	}

	// Handle tag format
	if strings.Contains(imageRef, ":") {
		// Split on last colon to handle IPv6 addresses in registry names
		lastColonIndex := strings.LastIndex(imageRef, ":")
		repoPath := imageRef[:lastColonIndex]
		ref.Tag = imageRef[lastColonIndex+1:]

		// Extract registry/repository from the part before ':'
		if strings.Contains(repoPath, "/") {
			// Check for possible registry prefix
			pathParts := strings.SplitN(repoPath, "/", 2)
			if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") || pathParts[0] == "localhost" {
				// This looks like a registry
				ref.Registry = pathParts[0]
				ref.Repository = pathParts[1]
			} else {
				// No registry, just a multi-part repository
				ref.Repository = repoPath
			}
		} else {
			// No registry specified and single-part repository
			ref.Repository = repoPath
		}

		// Apply Docker Hub library/ prefix for single name repositories
		if ref.Registry == DefaultRegistry && !strings.Contains(ref.Repository, "/") {
			ref.Repository = "library/" + ref.Repository
		}

		return ref, nil
	}

	// No tag or digest specified, just repository/registry
	if strings.Contains(imageRef, "/") {
		// Check for possible registry prefix
		pathParts := strings.SplitN(imageRef, "/", 2)
		if strings.Contains(pathParts[0], ".") || strings.Contains(pathParts[0], ":") || pathParts[0] == "localhost" {
			// This looks like a registry
			ref.Registry = pathParts[0]
			ref.Repository = pathParts[1]
		} else {
			// No registry, just a multi-part repository
			ref.Repository = imageRef
		}
	} else {
		// Single-part repository (e.g., "nginx")
		ref.Repository = imageRef
	}

	// Apply Docker Hub library/ prefix for single name repositories
	if ref.Registry == DefaultRegistry && !strings.Contains(ref.Repository, "/") {
		ref.Repository = "library/" + ref.Repository
	}

	// Set default tag or use AppVersion if available
	if len(chartMetadata) > 0 && chartMetadata[0] != nil && chartMetadata[0].AppVersion != "" {
		ref.Tag = chartMetadata[0].AppVersion
		log.Debug("Using Chart.AppVersion for tag: %s", ref.Tag)
	} else {
		ref.Tag = LatestTag
		log.Debug("Setting default tag: %s", ref.Tag)
	}

	return ref, nil
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
