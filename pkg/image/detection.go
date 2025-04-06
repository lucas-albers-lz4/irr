package image

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

// Constants for image detection
const (
	// Patterns for detecting image references
	tagPattern       = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9]):(?P<tag>[a-zA-Z0-9][-a-zA-Z0-9._]+)$`
	digestPattern    = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9])@(?P<digest>sha256:[a-fA-F0-9]{64})$`
	defaultRegistry  = "docker.io"
	libraryNamespace = "library"
)

var (
	// Precompiled regular expressions for image references
	tagRegex    = regexp.MustCompile(tagPattern)
	digestRegex = regexp.MustCompile(digestPattern)

	// Regex for basic validation (adjust as needed based on OCI spec)
	digestCharsRegex = regexp.MustCompile(`^[a-fA-F0-9]+$`) // Added for digest hex char validation
)

// ImageReference represents a container image reference
type ImageReference struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
	Path       []string // Path in the values structure where this reference was found
}

// LocationType defines how an image reference was structured in the original values.
type LocationType int

const (
	// TypeUnknown indicates an undetermined structure.
	TypeUnknown LocationType = iota
	// TypeMapRegistryRepositoryTag represents map{registry: "", repository: "", tag: ""}
	TypeMapRegistryRepositoryTag
	// TypeRepositoryTag represents map{repository: "", tag: ""} or map{image: ""} (if image contains repo+tag)
	TypeRepositoryTag
	// TypeString represents a simple string value like "repository:tag".
	TypeString
)

// DetectionContext holds configuration for image detection
type DetectionContext struct {
	SourceRegistries  []string
	ExcludeRegistries []string
	GlobalRegistry    string
	Strict            bool
	TemplateMode      bool
}

// DetectedImage represents an image found during detection
type DetectedImage struct {
	Reference    *ImageReference
	Location     []string
	LocationType LocationType
	Pattern      string      // "map", "string", "global"
	Original     interface{} // Original value (for template preservation)
}

// ImageDetector provides functionality for detecting images in values
type ImageDetector struct {
	context *DetectionContext
}

// String returns the string representation of the image reference
func (r *ImageReference) String() string {
	if r.Registry != "" {
		if r.Digest != "" {
			return fmt.Sprintf("%s/%s@%s", r.Registry, r.Repository, r.Digest)
		}
		return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, r.Tag)
	}

	if r.Digest != "" {
		return fmt.Sprintf("%s@%s", r.Repository, r.Digest)
	}
	return fmt.Sprintf("%s:%s", r.Repository, r.Tag)
}

// IsSourceRegistry checks if the image reference's registry matches any of the source registries
func IsSourceRegistry(ref *ImageReference, sourceRegistries []string, excludeRegistries []string) bool {
	debug.FunctionEnter("IsSourceRegistry")
	defer debug.FunctionExit("IsSourceRegistry")

	debug.DumpValue("Input Reference", ref)
	debug.DumpValue("Source Registries", sourceRegistries)
	debug.DumpValue("Exclude Registries", excludeRegistries)

	if ref == nil {
		debug.Println("Reference is nil, returning false")
		return false
	}

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

// NormalizeRegistry standardizes registry names for comparison
func NormalizeRegistry(registry string) string {
	if registry == "" {
		return defaultRegistry
	}

	// Convert to lowercase for consistent comparison
	registry = strings.ToLower(registry)

	// Handle docker.io special cases
	if registry == "docker.io" || registry == "index.docker.io" {
		return defaultRegistry
	}

	// Strip port number if present
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		registry = registry[:portIndex]
	}

	// Remove trailing slashes
	registry = strings.TrimSuffix(registry, "/")

	return registry
}

// SanitizeRegistryForPath makes a registry name safe for use in a path
func SanitizeRegistryForPath(registry string) string {
	// Replace potentially problematic characters
	// Remove port if present
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		registry = registry[:portIndex]
	}

	// Replace dots with empty string (remove them)
	sanitized := strings.ReplaceAll(registry, ".", "")

	// Replace slashes with dashes
	sanitized = strings.ReplaceAll(sanitized, "/", "-")

	// For docker.io, normalize to a standard "dockerio"
	if registry == "docker.io" || registry == "index.docker.io" {
		return "dockerio"
	}

	return sanitized
}

// Constants for image pattern types
const (
	PatternRepositoryTag   = "repository-tag"    // Map with repository and tag keys
	PatternRegistryRepoTag = "registry-repo-tag" // Map with registry, repository, and tag keys
	PatternImageString     = "image-string"      // Single string value (e.g., "nginx:latest")
	PatternUnsupported     = "unsupported"       // Unrecognized pattern
)

// Known path patterns for image-containing fields
var (
	imagePathPatterns = []string{
		"^image$",                 // key is exactly 'image'
		"\\bimage$",               // key ends with 'image'
		"^.*\\.image$",            // any path ending with '.image'
		"^.*\\.images\\[\\d+\\]$", // array elements in an 'images' array
		"^spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$",                      // k8s container image
		"^spec\\.template\\.spec\\.initContainers\\[\\d+\\]\\.image$",                  // k8s init container image
		"^spec\\.jobTemplate\\.spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$", // k8s job container image
	}

	// Compiled regex patterns for image paths
	imagePathRegexps = compilePathPatterns(imagePathPatterns)

	// Known non-image path patterns
	nonImagePathPatterns = []string{
		"\\.enabled$",
		"\\.annotations\\.",
		"\\.labels\\.",
		"\\.port$",
		"\\.ports\\.",
		"\\.timeout$",
		"\\.serviceAccountName$",
		"\\.replicas$",
		"\\.resources\\.",
		"\\.env\\.",
		"\\.command\\[\\d+\\]$",
		"\\.args\\[\\d+\\]$",
		"\\[\\d+\\]\\.name$",               // container name field
		"containers\\[\\d+\\]\\.name$",     // explicit k8s container name
		"initContainers\\[\\d+\\]\\.name$", // explicit k8s init container name
		"\\.tag$",                          // standalone tag field
		"\\.registry$",                     // standalone registry field
		"\\.repository$",                   // standalone repository field
	}

	// Compiled regex patterns for non-image paths
	nonImagePathRegexps = compilePathPatterns(nonImagePathPatterns)
)

// compilePathPatterns compiles a list of path patterns into regexps
func compilePathPatterns(patterns []string) []*regexp.Regexp {
	var regexps []*regexp.Regexp
	for _, pattern := range patterns {
		r := regexp.MustCompile(pattern)
		regexps = append(regexps, r)
	}
	return regexps
}

// NewImageDetector creates a new detector with optional context
func NewImageDetector(ctx *DetectionContext) *ImageDetector {
	if ctx == nil {
		ctx = &DetectionContext{}
	}
	return &ImageDetector{context: ctx}
}

// DetectImages finds all image references in a values structure
func (d *ImageDetector) DetectImages(values interface{}, path []string) ([]DetectedImage, []DetectedImage, error) {
	debug.FunctionEnter("DetectImages")
	defer debug.FunctionExit("DetectImages")

	var detected []DetectedImage
	var unsupported []DetectedImage

	// Handle nil values
	if values == nil {
		return detected, unsupported, nil
	}

	// Process global registry first if at root level
	if len(path) == 0 {
		if m, ok := values.(map[string]interface{}); ok {
			if global, ok := m["global"].(map[string]interface{}); ok {
				for k, v := range global {
					if strings.Contains(k, "registry") || strings.Contains(k, "Registry") {
						if str, ok := v.(string); ok {
							d.context.GlobalRegistry = str
						}
					}
				}
			}
		}
	}

	switch v := values.(type) {
	case map[string]interface{}:
		foundImageInMapStructure := false
		// Check for a simple "image: <string>" pattern first
		if imageVal, ok := v["image"]; ok {
			if imageStr, isString := imageVal.(string); isString {
				if ref, err := tryExtractImageFromString(imageStr); err == nil && ref != nil {
					// Found a valid image string under the "image" key
					if ref.Registry == "" && d.context.GlobalRegistry != "" {
						ref.Registry = d.context.GlobalRegistry
					}
					detected = append(detected, DetectedImage{
						Location:     append(path, "image"), // Path points to the specific key
						LocationType: TypeString,
						Reference:    ref,
						Pattern:      "string",
						Original:     imageStr,
					})
					// Note: We don't return early here anymore.
					// Mark that we found an image this way to avoid map structure detection later.
					foundImageInMapStructure = true
				}
			}
		}

		// If we didn't find an image via the simple "image" key,
		// try detecting the map structure itself (e.g., {repository: ..., tag: ...})
		if !foundImageInMapStructure {
			if ref, err := parseImageMap(v, d.context.GlobalRegistry); err != nil {
				return nil, nil, err // Propagate type errors
			} else if ref != nil {
				detected = append(detected, DetectedImage{
					Location:     path,
					LocationType: TypeMapRegistryRepositoryTag,
					Reference:    ref,
					Pattern:      "map",
					Original:     v,
				})
				// If detected as a map structure, don't recurse further into its keys
				return detected, unsupported, nil
			}
		}

		// Always recurse into map keys to find nested images,
		// unless the map itself was detected as an image structure above.
		for k, val := range v {
			// Skip the simple "image" key if we already processed it above
			if k == "image" && foundImageInMapStructure {
				continue
			}
			newPath := append(path, k)
			if len(path) == 0 && k == "global" {
				continue
			}
			subDetected, subUnsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				return nil, nil, fmt.Errorf("error processing key %s: %w", k, err)
			}
			detected = append(detected, subDetected...)
			unsupported = append(unsupported, subUnsupported...)
		}

	case []interface{}:
		for i, val := range v {
			newPath := append(path, fmt.Sprintf("[%d]", i))
			subDetected, subUnsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				return nil, nil, fmt.Errorf("error processing array index %d: %w", i, err)
			}
			detected = append(detected, subDetected...)
			unsupported = append(unsupported, subUnsupported...)
		}

	case string:
		// More selective processing of strings
		// Only process as potential image if:
		// 1. It's in a path that's likely to contain an image (like .spec.containers[0].image)
		// 2. Or it strictly matches an image reference pattern
		shouldProcess := isImagePath(path) || isStrictImageString(v)

		if shouldProcess {
			if ref, err := tryExtractImageFromString(v); err == nil && ref != nil {
				// Successfully parsed as an image
				// Apply global registry only if the parsed reference didn't have one
				parsedRegistry := ref.Registry
				if parsedRegistry == "" && d.context.GlobalRegistry != "" {
					ref.Registry = d.context.GlobalRegistry
				}
				detected = append(detected, DetectedImage{
					Location:     path,
					LocationType: TypeString,
					Reference:    ref,
					Pattern:      "string",
					Original:     v,
				})
			} else if d.context.Strict && strings.Contains(v, ":") && !strings.Contains(v, "//") {
				// In strict mode, if a string looks like an attempted image ref (has colon but no protocol)
				// but isn't a valid image reference, add it to unsupported
				unsupported = append(unsupported, DetectedImage{
					Location:     path,
					LocationType: TypeUnknown,
					Pattern:      "unsupported",
					Original:     v,
				})
			}
		}
	}

	return detected, unsupported, nil
}

// tryExtractImageFromMap attempts to extract an image reference from a map
func (d *ImageDetector) tryExtractImageFromMap(m map[string]interface{}) (*ImageReference, string, error) {
	// Get repository - required for an image map
	repoVal, repoExists := m["repository"]
	if !repoExists {
		debug.Printf("Map missing 'repository', cannot parse as image map.")
		return nil, "", ErrInvalidImageMapRepo // Return error if repository is missing
	}
	repoStr, repoIsString := repoVal.(string)
	if !repoIsString {
		debug.Printf("Map 'repository' exists but is not a string (type: %T).", repoVal)
		return nil, "", ErrInvalidImageMapRepo // Also return error if repo is wrong type
	}

	// Skip "repository" keys that don't look like image repositories
	// For example, Git repositories or other URLs that aren't container images
	if strings.HasPrefix(repoStr, "http") || strings.HasPrefix(repoStr, "git@") ||
		strings.HasSuffix(repoStr, ".git") || strings.Contains(repoStr, "github.com") {
		return nil, "", nil
	}

	// Proceed with creating the reference
	ref := &ImageReference{Repository: repoStr}

	// First check for explicit registry in the map
	if registryVal, exists := m["registry"]; exists {
		if registry, ok := registryVal.(string); ok {
			ref.Registry = registry
			ref.Repository = repoStr
		} else if registryVal != nil { // Check type only if the value is not nil
			return nil, "", ErrInvalidImageMapRegistryType
		}
		// If registry exists but is nil, it will be handled later (defaulted)
	}

	// If no explicit *valid* registry was found above, derive or use global/default
	if ref.Registry == "" {
		parts := strings.SplitN(repoStr, "/", 2)
		// Check if first part looks like a registry (contains dots) and is not a template variable
		if len(parts) == 2 && strings.Contains(parts[0], ".") && !strings.Contains(parts[0], "{{") {
			ref.Registry = parts[0]
			ref.Repository = parts[1]
		} else {
			// No registry found in repository string, use global or default
			if d.context.GlobalRegistry != "" {
				ref.Registry = d.context.GlobalRegistry
			} else {
				ref.Registry = defaultRegistry
			}
			ref.Repository = repoStr
		}
	}

	// Get tag
	if tagVal, exists := m["tag"]; exists {
		if tag, ok := tagVal.(string); ok {
			ref.Tag = tag
		} else if tagVal != nil { // Check type only if the value is not nil
			return nil, "", ErrInvalidImageMapTagType
		}
		// If tag exists but is nil, ref.Tag remains empty string, which is handled
	}

	// Get digest
	if digest, ok := m["digest"].(string); ok {
		ref.Digest = digest
	}

	// Normalize registry
	ref.Registry = NormalizeRegistry(ref.Registry)

	return normalizeImageReference(ref), "map", nil
}

// tryExtractImageFromString tries to extract an image reference from a string.
func tryExtractImageFromString(s string) (*ImageReference, error) {
	if s == "" {
		return nil, ErrEmptyImageString
	}

	// Try to parse as a Docker image reference
	ref := &ImageReference{}

	// Split by @ first to handle digest
	parts := strings.SplitN(s, "@", 2)
	if len(parts) == 2 {
		ref.Digest = parts[1]
		// Validate digest format - allow any length for flexibility
		// Allow sha256: prefix or just the hex chars for compatibility
		if !digestCharsRegex.MatchString(ref.Digest) && (!strings.HasPrefix(ref.Digest, "sha256:") || !digestCharsRegex.MatchString(strings.TrimPrefix(ref.Digest, "sha256:"))) {
			return nil, ErrInvalidDigestFormat
		}
		if !strings.HasPrefix(ref.Digest, "sha256:") {
			// Normalize digest to include prefix if missing
			ref.Digest = "sha256:" + ref.Digest
		}
	}

	// Remaining part might contain registry, repository, and tag
	repoAndTag := parts[0]

	// Find the last colon to potentially separate the tag
	lastColonIdx := strings.LastIndex(repoAndTag, ":")

	// Check if the colon exists and is not the first character
	if lastColonIdx > 0 {
		potentialTag := repoAndTag[lastColonIdx+1:]
		potentialRepoPart := repoAndTag[:lastColonIdx]

		// Validate the potential tag *immediately*
		if !isValidTag(potentialTag) {
			return nil, ErrInvalidTagFormat
		}

		// If the tag is valid, assign it and update the remaining part
		ref.Tag = potentialTag
		repoAndTag = potentialRepoPart
	} else if lastColonIdx == 0 {
		// Colon at the beginning is invalid (e.g., ":tag")
		return nil, ErrInvalidImageRefFormat
	}

	// Handle repository part from the remaining repoAndTag string
	repoStr := repoAndTag
	if strings.Contains(repoStr, "/") {
		// Has registry or organization
		repoParts := strings.SplitN(repoStr, "/", 2)
		if strings.Contains(repoParts[0], ".") || strings.Contains(repoParts[0], ":") || repoParts[0] == "localhost" {
			// First part contains . or : indicating it's a registry, or is localhost
			ref.Registry = repoParts[0]
			if !isValidRegistryName(ref.Registry) {
				return nil, ErrInvalidRegistryName
			}
			ref.Repository = repoParts[1]
		} else {
			// First part is an organization
			ref.Registry = defaultRegistry
			ref.Repository = repoStr
		}
	} else {
		// No registry or organization
		if !isValidDockerLibraryName(repoStr) {
			return nil, ErrInvalidRepoName
		}
		ref.Registry = defaultRegistry
		ref.Repository = fmt.Sprintf("library/%s", repoStr)
	}

	// Add default tag "latest" if no tag or digest was specified
	if ref.Tag == "" && ref.Digest == "" {
		ref.Tag = "latest"
	}

	// Additional validation to ensure this looks like an image reference
	// but with more lenient checks for automated tests
	if !isValidImageReference(ref) {
		// Special case for docker library images like "nginx" without tags
		if ref.Registry == defaultRegistry && strings.HasPrefix(ref.Repository, "library/") {
			return ref, nil
		}
		return nil, ErrInvalidImageRefFormat
	}

	return ref, nil
}

// ParseImageReference parses a string into an ImageReference
func ParseImageReference(input interface{}) (*ImageReference, error) {
	// Handle nil input
	if input == nil {
		return nil, nil
	}

	// Convert input to string
	str, ok := input.(string)
	if !ok {
		return nil, nil
	}

	// Handle empty string
	if str == "" {
		return nil, nil
	}

	// Parse the string into an ImageReference using the core logic
	ref, err := tryExtractImageFromString(str)

	// Handle errors from parsing
	if err != nil {
		return nil, err
	}

	// Defensive check in case tryExtractImageFromString returns nil, nil
	if ref == nil {
		return nil, nil
	}

	// Trust tryExtractImageFromString for validation, final checks removed.

	return ref, nil
}

// parseImageMap attempts to extract an ImageReference from a map[string]interface{}.
// It handles different patterns like {registry: ..., repository: ..., tag: ...}
// or {repository: ..., tag: ...} and applies global registry context if needed.
// It returns the ImageReference or nil if the map doesn't represent an image.
// It returns an error only if the map seems intended to be an image map but has invalid types.
func parseImageMap(m map[string]interface{}, globalRegistry string) (*ImageReference, error) {
	debug.Printf("Parsing image map: %v with global registry: %s", m, globalRegistry)

	ref := &ImageReference{}

	// Get repository - required for an image map
	repoVal, repoExists := m["repository"]
	if !repoExists {
		debug.Printf("Map missing 'repository', cannot parse as image map.")
		return nil, ErrInvalidImageMapRepo // Return error if repository is missing
	}
	repoStr, repoIsString := repoVal.(string)
	if !repoIsString {
		debug.Printf("Map 'repository' exists but is not a string (type: %T).", repoVal)
		return nil, ErrInvalidImageMapRepo // Also return error if repo is wrong type
	}

	// Check for explicit registry
	if registryVal, exists := m["registry"]; exists {
		if registryStr, ok := registryVal.(string); ok {
			debug.Printf("Found explicit registry in map: %s", registryStr)
			ref.Registry = registryStr
		} else if registryVal != nil { // Non-string, non-nil registry is an error
			debug.Printf("Map 'registry' exists but is not a string (type: %T).", registryVal)
			return nil, ErrInvalidImageMapRegistryType
		}
		// If registry exists but is nil, we treat it as missing and use default/global later
	}

	// If no explicit registry was set above, derive or use global/default
	if ref.Registry == "" {
		parts := strings.SplitN(repoStr, "/", 2)
		// Check if first part looks like a registry (contains dots/colon) and is not a template var
		if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) && !strings.Contains(parts[0], "{{") {
			debug.Printf("Derived registry '%s' from repository '%s'", parts[0], repoStr)
			ref.Registry = parts[0]
			ref.Repository = parts[1]
		} else {
			// No registry derived from repository string, use global or default
			if globalRegistry != "" {
				debug.Printf("Using global registry: %s", globalRegistry)
				ref.Registry = globalRegistry
			} else {
				debug.Printf("Using default registry: %s", defaultRegistry)
				ref.Registry = defaultRegistry
			}
			ref.Repository = repoStr // Repository is the original string
		}
	} else {
		// We had an explicit registry from the map, so the repository is just the repoStr
		ref.Repository = repoStr
	}

	// Get tag
	if tagVal, exists := m["tag"]; exists {
		if tagStr, ok := tagVal.(string); ok {
			ref.Tag = tagStr
		} else if tagVal != nil { // Non-string, non-nil tag is an error
			debug.Printf("Map 'tag' exists but is not a string (type: %T).", tagVal)
			return nil, ErrInvalidImageMapTagType
		}
		// If tag exists but is nil, ref.Tag remains empty string, which is valid
	}

	// Get digest
	if digestVal, exists := m["digest"]; exists {
		if digestStr, ok := digestVal.(string); ok {
			ref.Digest = digestStr
		} else if digestVal != nil {
			// Currently, we don't have a specific error for invalid digest type, maybe add later?
			debug.Printf("Map 'digest' exists but is not a string (type: %T), ignoring.", digestVal)
		}
	}

	// Normalize the potentially complex registry string before further processing
	ref.Registry = NormalizeRegistry(ref.Registry)
	debug.Printf("Normalized registry: %s", ref.Registry)

	// Apply library namespace logic AFTER registry normalization
	if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
		debug.Printf("Prepending 'library/' to repository '%s' for default registry", ref.Repository)
		ref.Repository = libraryNamespace + "/" + ref.Repository
	}

	debug.Printf("Parsed image map result: %+v", ref)
	return ref, nil
}

// normalizeImageReference ensures the reference has valid values
// DEPRECATED: Logic moved into parseImageMap and tryExtractImageFromString
func normalizeImageReference(ref *ImageReference) *ImageReference {
	// This function is largely redundant now as normalization happens within the parsing functions.
	// Keeping it for potential future use or complex normalization steps, but it's currently simple.
	if ref == nil {
		return nil
	}

	// Basic normalization: Ensure default registry if empty
	if ref.Registry == "" {
		ref.Registry = defaultRegistry
	}

	// The library namespace logic is now handled in parseImageMap and tryExtractImageFromString
	// where context (like the original map/string and global registry) is available.

	return ref
}

// isValidTag checks if a tag string is valid
func isValidTag(tag string) bool {
	if tag == "" {
		return false
	}

	// Maximum length for a tag
	if len(tag) > 128 {
		return false
	}

	// Tag cannot contain slashes
	if strings.Contains(tag, "/") {
		return false
	}

	for _, c := range tag {
		// nolint:staticcheck // Intentionally keeping complex boolean logic for readability
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// isValidImageReference checks if an image reference is valid
func isValidImageReference(ref *ImageReference) bool {
	if ref == nil {
		return false
	}

	// Check registry name
	if !isValidRegistryName(ref.Registry) {
		return false
	}

	// Check repository
	if !isValidRepositoryPart(ref.Repository) {
		return false
	}

	// Check tag and digest
	if ref.Tag != "" && ref.Digest != "" {
		return false
	}
	if ref.Tag != "" && !isValidTag(ref.Tag) {
		return false
	}
	if ref.Digest != "" && !strings.HasPrefix(ref.Digest, "sha256:") {
		return false
	}

	return true
}

// isValidRegistryName checks if a registry name is valid
func isValidRegistryName(name string) bool {
	if name == "" {
		return false
	}

	// Handle special cases
	if name == "docker.io" || name == "localhost" {
		return true
	}

	// Handle registry with port
	if strings.Contains(name, ":") {
		parts := strings.Split(name, ":")
		if len(parts) != 2 {
			return false
		}
		name = parts[0] // Check only the hostname
	}

	// Check domain-like format
	parts := strings.Split(name, ".")
	if len(parts) < 2 && name != "localhost" {
		return false
	}

	// Maximum of 3 parts in a domain name (e.g., registry.example.com)
	if len(parts) > 3 {
		return false
	}

	for _, part := range parts {
		if !isValidDomainPart(part) {
			return false
		}
	}
	return true
}

// isValidDomainPart checks if a domain name part is valid
func isValidDomainPart(part string) bool {
	if part == "" {
		return false
	}
	for _, c := range part {
		// nolint:staticcheck // Intentionally keeping complex boolean logic for readability
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// isValidDockerLibraryName checks if a name is valid for the Docker library
func isValidDockerLibraryName(name string) bool {
	if name == "" || strings.Contains(name, "/") {
		return false
	}
	for _, c := range name {
		// nolint:staticcheck // Intentionally keeping complex boolean logic for readability
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// isValidRepositoryPart checks if a repository name is valid
func isValidRepositoryPart(repo string) bool {
	if repo == "" {
		return false
	}

	// Maximum length for a repository name
	if len(repo) > 255 {
		return false
	}

	// Maximum of 5 parts in a repository path (e.g., org/suborg/group/subgroup/app)
	parts := strings.Split(repo, "/")
	if len(parts) > 5 {
		return false
	}

	// Repository must be lowercase and can contain alphanumeric characters, dots, dashes, and slashes
	// Each part between slashes must be valid
	for _, part := range parts {
		if part == "" {
			return false
		}
		if !isValidNamePart(part) {
			return false
		}
	}
	return true
}

// isValidNamePart checks if a single part of a name is valid
func isValidNamePart(part string) bool {
	if part == "" {
		return false
	}
	// Must start and end with alphanumeric character
	if !isAlphanumeric(rune(part[0])) || !isAlphanumeric(rune(part[len(part)-1])) {
		return false
	}
	// Can contain lowercase alphanumeric characters, dots, and dashes
	for _, r := range part {
		if !isAlphanumeric(r) && r != '.' && r != '-' {
			return false
		}
	}
	// Check for consecutive dots or dashes
	if strings.Contains(part, "..") || strings.Contains(part, "--") {
		return false
	}
	return true
}

// isAlphanumeric checks if a rune is a lowercase letter or number
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

// isImagePath checks if a path is likely to contain an image reference
func isImagePath(path []string) bool {
	// Short circuit for empty paths
	if len(path) == 0 {
		return false
	}

	// Special check for direct 'image' key
	if len(path) > 0 && path[len(path)-1] == "image" {
		return true
	}

	// Convert path to string for regex matching
	pathStr := strings.Join(path, ".")

	// Check against known image path patterns
	for _, pattern := range imagePathRegexps {
		if pattern.MatchString(pathStr) {
			return true
		}
	}

	// Check against known non-image path patterns
	for _, pattern := range nonImagePathRegexps {
		if pattern.MatchString(pathStr) {
			return false
		}
	}

	// Default to false for unknown paths
	return false
}

// isStrictImageString checks if a string strictly matches image reference patterns
func isStrictImageString(s string) bool {
	// Check for tag-based reference
	if tagRegex.MatchString(s) {
		return true
	}

	// Check for digest-based reference
	if digestRegex.MatchString(s) {
		return true
	}

	return false
}

// DetectImages finds all image references in a values structure
func DetectImages(values interface{}, path []string, sourceRegistries []string, excludeRegistries []string, strict bool) ([]DetectedImage, []DetectedImage, error) {
	detector := NewImageDetector(&DetectionContext{
		SourceRegistries:  sourceRegistries,
		ExcludeRegistries: excludeRegistries,
		Strict:            strict,
	})
	return detector.DetectImages(values, path)
}

// Package image provides functionality for detecting and manipulating container image references.
// ... existing code ...
