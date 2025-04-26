package image

import (
	"fmt"
	"regexp"
	"strings"

	distref "github.com/distribution/reference"
	"github.com/lucas-albers-lz4/irr/pkg/log"
)

const (
	defaultRegistry  = "docker.io"
	libraryNamespace = "library"
	defaultTag       = "latest"
)

// Define a simple regex to check if a string looks like a potential port number
var portRegex = regexp.MustCompile(`^\d+$`)

// NormalizeRegistry standardizes registry names for comparison
func NormalizeRegistry(registry string) string {
	// Trim leading/trailing whitespace and control characters (like \r)
	trimmedRegistry := strings.TrimSpace(registry)
	if trimmedRegistry == "" {
		return defaultRegistry
	}

	// Convert to lowercase for consistent comparison
	lowerRegistry := strings.ToLower(trimmedRegistry)

	// Handle docker.io special cases EARLY, before path/port stripping
	if lowerRegistry == defaultRegistry || lowerRegistry == "index.docker.io" {
		return defaultRegistry
	}

	// Separate hostname from potential path/port
	hostname := lowerRegistry
	firstSlash := strings.Index(hostname, "/")
	if firstSlash != -1 {
		hostname = hostname[:firstSlash]
		log.Debug("NormalizeRegistry: Stripped path component from '%s', result: '%s'", lowerRegistry, hostname)
	}

	// Strip port number from the hostname part if present
	if portIndex := strings.LastIndex(hostname, ":"); portIndex != -1 {
		potentialPort := hostname[portIndex+1:]
		// Use regex to ensure it's only digits
		if portRegex.MatchString(potentialPort) {
			log.Debug("NormalizeRegistry: Stripped port '%s' from hostname '%s'", potentialPort, hostname)
			hostname = hostname[:portIndex]
		} else {
			log.Debug("NormalizeRegistry: ':' found in hostname '%s' but part after it ('%s') is not numeric, not stripping.", hostname, potentialPort)
		}
	}

	// Note: No need to remove trailing slashes as path component is already removed.

	log.Debug("NormalizeRegistry: Input '%s' -> Normalized '%s'", registry, hostname)
	return hostname
}

// SanitizeRegistryForPath makes a registry name safe for use in a path component.
// It primarily removes dots and ports.
func SanitizeRegistryForPath(registry string) string {
	// Handle docker.io special case first - it retains the dot
	if registry == defaultRegistry || registry == "index.docker.io" {
		return defaultRegistry // Return 'docker.io' directly
	}

	// Strip port number if present
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		potentialPort := registry[portIndex+1:]
		if _, err := fmt.Sscan(potentialPort, new(int)); err == nil {
			registry = registry[:portIndex]
		} else {
			log.Debug("SanitizeRegistryForPath: ':' found in '%s' but part after it ('%s') "+
				"is not numeric, not treating as port.", registry, potentialPort)
		}
	}

	// Remove dots
	sanitized := strings.ReplaceAll(registry, ".", "")

	// DO NOT replace slashes

	// DO NOT add port back

	return sanitized
}

// IsSourceRegistry checks if the image reference's registry matches any of the source registries
func IsSourceRegistry(ref *Reference, sourceRegistries, excludeRegistries []string) bool {
	// Check for nil ref immediately to prevent panic in deferred debug calls.
	if ref == nil {
		log.Debug("IsSourceRegistry called with nil Reference, returning false")
		return false
	}

	// Now ref is known non-nil, proceed with debug setup.
	log.Debug("Enter IsSourceRegistry")
	defer log.Debug("Exit IsSourceRegistry")

	log.Debug("Input Reference", "value", ref)
	log.Debug("Source Registries", "value", sourceRegistries)
	log.Debug("Exclude Registries", "value", excludeRegistries)

	// Normalize registry names for comparison
	registry := NormalizeRegistry(ref.Registry)
	log.Debug("Normalized registry name", "value", registry)

	// Check if the registry is in the exclusion list
	for _, exclude := range excludeRegistries {
		excludeNorm := NormalizeRegistry(exclude)
		log.Debug("Checking against excluded registry", "value", exclude, "normalized", excludeNorm)
		if registry == excludeNorm {
			log.Debug("Registry %s is excluded", registry)
			return false
		}
	}

	// Check if the registry matches any of the source registries
	for _, source := range sourceRegistries {
		sourceNorm := NormalizeRegistry(source)
		log.Debug("Checking against source registry", "value", source, "normalized", sourceNorm)
		if registry == sourceNorm {
			log.Debug("Registry %s matches source %s", registry, source)
			return true
		}
	}

	log.Debug("Registry %s does not match any source registries", registry)
	return false
}

// NormalizeImageReference applies normalization rules (default registry, default tag, library namespace)
// to a parsed ImageReference in place, leveraging the distribution/reference library.
func NormalizeImageReference(ref *Reference) {
	if ref == nil {
		return
	}

	log.Debug("Enter NormalizeImageReference")
	defer log.Debug("Exit NormalizeImageReference")

	// We'll use the distribution/reference library for normalization
	// First, construct a reference string based on current values
	var refStr string
	if ref.Digest != "" {
		refStr = ref.Registry + "/" + ref.Repository + "@" + ref.Digest
		log.Debug("Constructed digest-based ref string", "value", refStr)
	} else {
		// If tag is empty, we'll let ParseNormalizedNamed add the default
		if ref.Tag == "" {
			refStr = ref.Registry + "/" + ref.Repository
			log.Debug("Constructed ref string without tag (for implicit latest)", "value", refStr)
		} else {
			refStr = ref.Registry + "/" + ref.Repository + ":" + ref.Tag
			log.Debug("Constructed tag-based ref string", "value", refStr)
		}
	}

	// Parse using the library's normalization function
	named, err := distref.ParseNormalizedNamed(refStr)
	if err != nil {
		// If there's a parsing error, fall back to manual normalization
		log.Debug("Warning: ParseNormalizedNamed failed for '%s'", "value", refStr)
		log.Debug("Falling back to manual normalization")

		// 1. Default Registry
		if ref.Registry == "" {
			ref.Registry = defaultRegistry
			log.Debug("Normalized: Registry defaulted to %s", defaultRegistry)
		} else {
			// Normalize existing registry name (lowercase, handle index.docker.io, strip port/suffix)
			ref.Registry = NormalizeRegistry(ref.Registry)
			log.Debug("Normalized: Registry processed to %s", ref.Registry)
		}

		// 2. Default Tag (only if no digest)
		if ref.Tag == "" && ref.Digest == "" {
			ref.Tag = defaultTag
			log.Debug("Normalized: Tag defaulted to latest")
		}

		// 3. Add "library/" namespace for docker.io if repository has no slashes
		if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
			ref.Repository = libraryNamespace + "/" + ref.Repository
			log.Debug("Normalized: Added '%s/' prefix to repository", "value", libraryNamespace, "repository", ref.Repository)
		}
	} else {
		// Successful parsing - use the normalized components from the library
		log.Debug("Successfully normalized to", "value", named.String())

		// Extract normalized components
		ref.Registry = distref.Domain(named)
		ref.Repository = distref.Path(named)

		// Extract tag/digest
		if taggedRef, isTagged := named.(distref.Tagged); isTagged {
			ref.Tag = taggedRef.Tag()
			log.Debug("Normalized tag", "value", ref.Tag)
		} else if ref.Digest == "" {
			// If neither tag nor digest, set default tag
			ref.Tag = defaultTag
			log.Debug("No tag or digest after normalization, defaulting to %s", defaultTag)
		}

		if digestedRef, isDigested := named.(distref.Digested); isDigested {
			ref.Digest = digestedRef.Digest().String()
			log.Debug("Normalized digest", "digest", ref.Digest)
		}
	}

	// Ensure Original is set if not already (should be set by parser, but safeguard)
	if ref.Original == "" {
		ref.Original = ref.String()
		log.Debug("Original field was empty, set to reconstructed string", "original", ref.Original)
	}
}
