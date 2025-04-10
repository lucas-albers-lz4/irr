package image

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
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
		debug.Printf("NormalizeRegistry: Stripped path component from '%s', result: '%s'", lowerRegistry, hostname)
	}

	// Strip port number from the hostname part if present
	if portIndex := strings.LastIndex(hostname, ":"); portIndex != -1 {
		potentialPort := hostname[portIndex+1:]
		// Use regex to ensure it's only digits
		if portRegex.MatchString(potentialPort) {
			debug.Printf("NormalizeRegistry: Stripped port '%s' from hostname '%s'", potentialPort, hostname)
			hostname = hostname[:portIndex]
		} else {
			debug.Printf("NormalizeRegistry: ':' found in hostname '%s' but part after it ('%s') is not numeric, not stripping.", hostname, potentialPort)
		}
	}

	// Note: No need to remove trailing slashes as path component is already removed.

	debug.Printf("NormalizeRegistry: Input '%s' -> Normalized '%s'", registry, hostname)
	return hostname
}

// SanitizeRegistryForPath makes a registry name safe for use in a path component.
// It primarily removes dots and ports.
func SanitizeRegistryForPath(registry string) string {
	// Normalize docker.io variants first
	if registry == defaultRegistry || registry == "index.docker.io" || registry == "" {
		return "dockerio"
	}

	// Strip port number if present
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		potentialPort := registry[portIndex+1:]
		if _, err := fmt.Sscan(potentialPort, new(int)); err == nil {
			registry = registry[:portIndex]
		} else {
			debug.Printf("SanitizeRegistryForPath: ':' found in '%s' but part after it ('%s') "+
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
		debug.Println("IsSourceRegistry called with nil Reference, returning false")
		return false
	}

	// Now ref is known non-nil, proceed with debug setup.
	debug.FunctionEnter("IsSourceRegistry")
	defer debug.FunctionExit("IsSourceRegistry")

	debug.DumpValue("Input Reference", ref)
	debug.DumpValue("Source Registries", sourceRegistries)
	debug.DumpValue("Exclude Registries", excludeRegistries)

	// Normalize registry names for comparison
	registry := NormalizeRegistry(ref.Registry)
	debug.Printf("Normalized registry name: %s", registry)

	// Check if the registry is in the exclusion list
	for _, exclude := range excludeRegistries {
		excludeNorm := NormalizeRegistry(exclude)
		debug.Printf("Checking against excluded registry: %s (normalized: %s)", exclude, excludeNorm)
		if registry == excludeNorm {
			debug.Printf("Registry %s is excluded", registry)
			return false
		}
	}

	// Check if the registry matches any of the source registries
	for _, source := range sourceRegistries {
		sourceNorm := NormalizeRegistry(source)
		debug.Printf("Checking against source registry: %s (normalized: %s)", source, sourceNorm)
		if registry == sourceNorm {
			debug.Printf("Registry %s matches source %s", registry, source)
			return true
		}
	}

	debug.Printf("Registry %s does not match any source registries", registry)
	return false
}

// NormalizeImageReference applies normalization rules (default registry, default tag, library namespace)
// to a parsed ImageReference in place.
func NormalizeImageReference(ref *Reference) {
	if ref == nil {
		return
	}

	debug.FunctionEnter("NormalizeImageReference")
	defer debug.FunctionExit("NormalizeImageReference")

	// 1. Default Registry
	if ref.Registry == "" {
		ref.Registry = defaultRegistry
		debug.Printf("Normalized: Registry defaulted to %s", defaultRegistry)
	} else {
		// Normalize existing registry name (lowercase, handle index.docker.io, strip port/suffix)
		ref.Registry = NormalizeRegistry(ref.Registry)
		debug.Printf("Normalized: Registry processed to %s", ref.Registry)
	}

	// 2. Default Tag (only if no digest)
	if ref.Tag == "" && ref.Digest == "" {
		ref.Tag = defaultTag
		debug.Printf("Normalized: Tag defaulted to latest")
	}

	// 3. Add "library/" namespace for docker.io if repository has no slashes
	if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
		ref.Repository = libraryNamespace + "/" + ref.Repository
		debug.Printf("Normalized: Added '%s/' prefix to repository: %s", libraryNamespace, ref.Repository)
	}

	// Ensure Original is set if not already (should be set by parser, but safeguard)
	if ref.Original == "" {
		ref.Original = ref.String()
		debug.Printf("Normalized: Original field was empty, set to reconstructed string: %s", ref.Original)
	}
}
