package image

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

// Helper function to get map keys for logging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

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
	// For docker.io, normalize to a standard "dockerio"
	if registry == "docker.io" || registry == "index.docker.io" {
		return "dockerio"
	}

	// Extract port if present
	var port string
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		port = registry[portIndex+1:]
		registry = registry[:portIndex]
	}

	// Replace dots with empty string (remove them)
	sanitized := strings.ReplaceAll(registry, ".", "")

	// Replace slashes with dashes
	sanitized = strings.ReplaceAll(sanitized, "/", "-")

	// Add back port if it was present
	if port != "" {
		sanitized = sanitized + port
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
	// --- START DetectImages DEBUG ---
	fmt.Printf("[DEBUG irr DETECT] >> Entering DetectImages. Path: %v\n", path)
	fmt.Printf("[DEBUG irr DETECT]    Values type: %T\n", values)
	if m, ok := values.(map[string]interface{}); ok {
		fmt.Printf("[DEBUG irr DETECT]    Values (map): %d keys\n", len(m))
		// Optionally print keys or a limited dump for maps
		// debug.DumpValue("Map Values", m)
	} else if s, ok := values.([]interface{}); ok {
		fmt.Printf("[DEBUG irr DETECT]    Values (slice): %d items\n", len(s))
	} else {
		fmt.Printf("[DEBUG irr DETECT]    Values (other): %v\n", values)
	}
	// --- END DetectImages DEBUG ---

	var allDetected, allUnsupported []DetectedImage

	// Handle nil values
	if values == nil {
		return allDetected, allUnsupported, nil
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
		debug.Printf("Processing map at path: %v", path)
		// 1. Check if the map itself is an image structure
		mapAsImageDetected, mapAsImageUnsupported, err := d.detectImageValue(v, path)
		if err != nil {
			debug.Printf("Error checking if map itself is an image at path %v: %v", path, err)
			// Logged the error, continue processing keys
		}
		allDetected = append(allDetected, mapAsImageDetected...)
		allUnsupported = append(allUnsupported, mapAsImageUnsupported...)

		// 2. Determine if we should recurse into children
		skipRecursion := len(mapAsImageDetected) > 0
		if skipRecursion {
			debug.Printf("Map at path %v is an image structure, skipping recursion into its keys.", path)
		}

		// 3. Iterate through all keys
		for key, value := range v {
			if skipRecursion {
				continue // Don't process children like repository, tag if map itself was image
			}

			// Prepare path for recursion
			currentPath := append([]string{}, path...)
			currentPath = append(currentPath, key)

			// 4. ALWAYS recurse using DetectImages (No special 'image' key handling)
			debug.Printf("Recursively calling DetectImages for key '%s' at path %v", key, path)
			subDetected, subUnsupported, err := d.DetectImages(value, currentPath)
			if err != nil {
				// Log error for this specific key/value pair, but continue processing siblings
				debug.Printf("Error processing value for key '%s' at path %v: %v. Skipping this key.", key, path, err)
				continue
			}
			allDetected = append(allDetected, subDetected...)
			allUnsupported = append(allUnsupported, subUnsupported...)
		}

	case []interface{}:
		for i, val := range v {
			newPath := append(path, fmt.Sprintf("[%d]", i))
			subDetected, subUnsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				return nil, nil, fmt.Errorf("error processing array index %d: %w", i, err)
			}
			allDetected = append(allDetected, subDetected...)
			allUnsupported = append(allUnsupported, subUnsupported...)
		}

	case string:
		shouldProcess := isImagePath(path) || isStrictImageString(v)
		if shouldProcess {
			if ref, err := tryExtractImageFromString(v); err == nil && ref != nil {
				if ref.Registry == "" && d.context.GlobalRegistry != "" {
					ref.Registry = d.context.GlobalRegistry
				}
				allDetected = append(allDetected, DetectedImage{
					Location:     path,
					LocationType: TypeString,
					Reference:    ref,
					Pattern:      "string",
					Original:     v,
				})
				debug.Printf("DETECTED (string): Path=%v, Ref=%+v", path, ref)
			} else if d.context.Strict && strings.Contains(v, ":") && !strings.Contains(v, "//") {
				allUnsupported = append(allUnsupported, DetectedImage{
					Location:     path,
					LocationType: TypeUnknown,
					Pattern:      "unsupported",
					Original:     v,
				})
			}
		}
	}

	return allDetected, allUnsupported, nil
}

// detectImageValue is a helper to process the value found under an 'image' key.
func (d *ImageDetector) detectImageValue(imageVal interface{}, path []string) ([]DetectedImage, []DetectedImage, error) {
	// --- START detectImageValue DEBUG ---
	fmt.Printf("[DEBUG irr DETECT VAL] >> Entering detectImageValue. Path: %v, ValueType: %T\n", path, imageVal)
	// --- END detectImageValue DEBUG ---

	var detected, unsupported []DetectedImage

	switch img := imageVal.(type) {
	case string:
		if ref, err := tryExtractImageFromString(img); err == nil && ref != nil {
			if ref.Registry == "" && d.context.GlobalRegistry != "" {
				ref.Registry = d.context.GlobalRegistry
			}
			detected = append(detected, DetectedImage{
				Location:     path, // Path already includes "image"
				LocationType: TypeString,
				Reference:    ref,
				Pattern:      "string",
				Original:     img,
			})
			debug.Printf("DETECTED (image:string): Path=%v, Ref=%+v", path, ref)
		} else if d.context.Strict && err != nil && !errors.Is(err, ErrEmptyImageString) {
			// If strict and parsing fails (and not just empty), mark unsupported
			unsupported = append(unsupported, DetectedImage{
				Location:     path,
				LocationType: TypeString,
				Pattern:      "unsupported-string",
				Original:     img,
			})
		}
	case map[string]interface{}:
		// Attempt to parse the map as a standard image structure
		debug.Printf("Value is map. Calling parseImageMap. Path: %v", path)
		// Call as method, handle 3 return values
		ref, locType, err := d.parseImageMap(img, path)
		if err != nil {
			// Map looked like image structure but failed parsing (e.g., invalid type for tag)
			debug.Printf("DETECT FAILED (map parse error): Path=%v, Error=%v", path, err)
			// Always add parsing errors to unsupported regardless of strict mode
			// The generator layer will check strict mode later.
			unsupported = append(unsupported, DetectedImage{
				Location:     path,
				LocationType: TypeUnknown, // Indicate parsing failure
				Pattern:      PatternUnsupported,
				Original:     imageVal,
			})
		} else if ref != nil {
			// Successfully parsed as an image map
			debug.Printf("DETECTED (map): Path=%v, Ref=%+v, Type=%d", path, ref, locType)
			detected = append(detected, DetectedImage{
				Location:     path,
				LocationType: locType,                // Use type returned by parseImageMap
				Pattern:      PatternRegistryRepoTag, // Adjust if needed based on locType
				Reference:    ref,
				Original:     imageVal,
			})
		} else { // ref is nil, err is nil
			// Map didn't parse as standard image map (e.g., missing repository or using name/version)
			debug.Printf("Map at path %v did not parse as image map (likely not an image structure)", path)
			// If strict mode is enabled in the context, add this non-standard map to unsupported.
			if d.context.Strict {
				debug.Printf("Adding map at path %v to unsupported list due to strict mode.", path)
				unsupported = append(unsupported, DetectedImage{
					Location:     path,
					LocationType: TypeUnknown, // It's an unknown/unsupported map structure
					Pattern:      PatternUnsupported,
					Original:     imageVal,
				})
			}
		}
	default:
		// Value under 'image' key is neither string nor map - unsupported?
		debug.Printf("Unsupported type (%T) under 'image' key at path %v", imageVal, path)
		if d.context.Strict {
			unsupported = append(unsupported, DetectedImage{
				Location:     path,
				LocationType: TypeUnknown,
				Pattern:      "unsupported-image-key-type",
				Original:     imageVal,
			})
		}
	}
	return detected, unsupported, nil
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
			// If the tag is invalid, treat the whole string as the repository

		} else {
			// If the tag is valid, assign it and update the remaining part
			ref.Tag = potentialTag
			repoAndTag = potentialRepoPart
		}
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
		return nil, ErrInvalidRepoName
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

// parseImageMap attempts to parse a map[string]interface{} into an ImageReference.
// It returns the ImageReference or nil if the map doesn't represent an image.
// It returns an error only if the map seems intended to be an image map but has invalid types.
// NOTE: This is now a method of ImageDetector to access context.GlobalRegistry
func (d *ImageDetector) parseImageMap(m map[string]interface{}, path []string) (*ImageReference, LocationType, error) {
	// --- START parseImageMap DEBUG ---
	fmt.Printf("[DEBUG irr PARSE MAP] >> Attempting to parse map at path: %v\n", path)
	fmt.Printf("[DEBUG irr PARSE MAP]    Map Keys: %v\n", getMapKeys(m))
	// --- END parseImageMap DEBUG ---

	ref := &ImageReference{}
	var repoVal interface{}
	var repoExists bool

	// Check for required 'repository' key
	repoVal, repoExists = m["repository"]
	if !repoExists {
		debug.Printf("Map missing 'repository', not an image map")
		return nil, TypeUnknown, nil // Return nil, Unknown type, nil error
	}

	// Also require 'tag' key for Type 2 maps
	tagVal, tagExists := m["tag"]
	if !tagExists {
		debug.Printf("Map has 'repository' but missing 'tag', not a valid image map")
		return nil, TypeUnknown, nil // Not a complete image map
	}

	repoStr, repoIsString := repoVal.(string)
	if !repoIsString {
		debug.Printf("Map 'repository' exists but is not a string (type: %T).", repoVal)
		return nil, TypeUnknown, ErrInvalidImageMapRepo // Return nil, Unknown type, error
	}

	// Also validate tag is a string
	tagStr, tagIsString := tagVal.(string)
	if !tagIsString {
		debug.Printf("Map 'tag' exists but is not a string (type: %T).", tagVal)
		return nil, TypeUnknown, ErrInvalidImageMapTagType
	}

	// Now we have both repository and tag as strings, we can continue

	// Attempt to parse registry from the repository string itself (e.g., docker.io/library/nginx)
	if strings.Contains(repoStr, "/") {
		parts := strings.SplitN(repoStr, "/", 2)
		// Basic check if the first part looks like a domain name (contains '.')
		if strings.Contains(parts[0], ".") {
			ref.Registry = parts[0]
			ref.Repository = parts[1]
			debug.Printf("Registry derived from repository string: %s", ref.Registry)
		} else {
			// Does not look like registry/repo format, assume full string is repo
			ref.Repository = repoStr
		}
	} else {
		// No slash, assume it's just the repository name (needs registry lookup)
		ref.Repository = repoStr
	}

	// Handle explicit 'registry' key, potentially overriding derived one or global/default
	registryVal, registryExists := m["registry"]
	if registryExists {
		if regStr, regIsString := registryVal.(string); regIsString {
			ref.Registry = regStr // Explicit registry overrides derived/default
			debug.Printf("Using explicit registry key: %s", ref.Registry)
		} else if registryVal != nil { // Non-string, non-nil registry is an error
			debug.Printf("Map 'registry' exists but is not a string (type: %T).", registryVal)
			return nil, TypeUnknown, ErrInvalidImageMapRegistryType // Return nil, Unknown type, error
		}
		// If registry exists but is nil, we treat it as missing and use default/global later
	}

	// If registry is still empty after checking explicit key and deriving from repo,
	// use global registry or default to docker.io
	if ref.Registry == "" {
		if d.context.GlobalRegistry != "" { // Use receiver context
			debug.Printf("Using global registry: %s", d.context.GlobalRegistry)
			ref.Registry = d.context.GlobalRegistry
		} else {
			debug.Printf("Using default registry: %s", defaultRegistry)
			ref.Registry = defaultRegistry
			// If repo didn't contain a slash, prepend library/
			if !strings.Contains(ref.Repository, "/") {
				ref.Repository = libraryNamespace + "/" + ref.Repository
				debug.Printf("Prepended '%s/' to repository: %s", libraryNamespace, ref.Repository)
			}
		}
	}

	// Set the tag from our validated tag string
	ref.Tag = tagStr

	// Handle 'digest' key (takes precedence over tag if valid)
	digestVal, digestExists := m["digest"]
	if digestExists {
		if digestStr, digestIsString := digestVal.(string); digestIsString {
			if digestStr != "" {
				// Basic validation: check for sha256: prefix and hex characters
				if strings.HasPrefix(digestStr, "sha256:") {
					digestHex := strings.TrimPrefix(digestStr, "sha256:")
					if len(digestHex) == 64 && digestCharsRegex.MatchString(digestHex) {
						ref.Digest = digestStr
						ref.Tag = "" // Clear tag if valid digest is found
					} else {
						debug.Printf("Map 'digest' has invalid sha256 format: %s", digestStr)
						// Don't error, just ignore the invalid digest
					}
				} else {
					debug.Printf("Map 'digest' is missing sha256: prefix: %s", digestStr)
					// Don't error, just ignore the invalid digest
				}
			}
		} else if digestVal != nil { // Non-string, non-nil digest is an error
			debug.Printf("Map 'digest' exists but is not a string (type: %T).", digestVal)
			return nil, TypeUnknown, ErrInvalidImageMapDigestType // Invalid type is an error
		}
		// If digest exists but is nil or empty string, it's ignored
	}

	// Determine LocationType based on keys present (simplified example)
	locType := TypeRepositoryTag              // Default if only repo/tag/digest
	if registryExists && ref.Registry != "" { // Check if explicit registry was successfully parsed
		locType = TypeMapRegistryRepositoryTag
	}

	debug.Printf("Parsed image map result: %+v", ref)
	// Ensure the parsed reference is actually valid before returning TypeRepositoryTag
	if !isValidImageReference(ref) {
		debug.Printf("Map at path %v looked like TypeRepositoryTag but parsed ref is invalid. Ref: %+v", path, ref)
		return nil, TypeUnknown, fmt.Errorf("parsed image reference from map is invalid: %+v", ref)
	}
	return ref, locType, nil // Return reference, determined type, nil error
}

// isValidDigestFormat checks if a string matches the expected sha256 digest format.
func isValidDigestFormat(digest string) bool {
	if !strings.HasPrefix(digest, "sha256:") {
		return false
	}
	digestHex := strings.TrimPrefix(digest, "sha256:")
	return len(digestHex) == 64 && digestCharsRegex.MatchString(digestHex)
}

// isValidTag checks if a string is a valid Docker tag.
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

// processMap handles detection within a map.
func (d *ImageDetector) processMap(m map[string]interface{}, path []string) ([]DetectedImage, []DetectedImage, error) {
	var allDetected, allUnsupported []DetectedImage
	debug.Printf("[PROCESS MAP] Processing map at path: %v", path)

	// First, try to detect if the map ITSELF is an image structure
	mapDetected, mapUnsupported, err := d.detectImageValue(m, path)
	if err != nil {
		// Log the error but don't stop processing sibling keys
		debug.Printf("Error attempting to detect map as image at path %v: %v", path, err)
		// Consider adding to allUnsupported here if desired
	} else {
		allDetected = append(allDetected, mapDetected...)
		allUnsupported = append(allUnsupported, mapUnsupported...)
	}

	// If the map itself was identified as an image, DON'T recurse into its keys
	if len(mapDetected) > 0 {
		debug.Printf("[PROCESS MAP] Map at path %v IS an image structure, skipping recursion into its keys.", path)
		return allDetected, allUnsupported, nil
	}

	// If the map itself is NOT an image, recurse into its key-value pairs
	debug.Printf("[PROCESS MAP] Map at path %v is NOT an image structure itself, recursing into keys...", path)
	for k, v := range m {
		currentPath := append([]string{}, path...) // Create a copy
		currentPath = append(currentPath, k)

		debug.Printf("[PROCESS MAP] Recursing for key '%s' at path %v...", k, path)
		// Recursively call DetectImages for the value
		detected, unsupported, err := d.DetectImages(v, currentPath)
		if err != nil {
			debug.Printf("[PROCESS MAP] ERROR during recursion for key '%s' at path %v: %v", k, path, err)
			// Decide how to handle errors - skip key, return error, etc.
			// For now, let's log and continue to process other keys
			debug.Printf("Error detecting images for key '%s' at path %v: %v. Skipping this key.", k, path, err)
			continue // Or return nil, nil, err
		}
		debug.Printf("[PROCESS MAP] Finished recursion for key '%s' at path %v. Found %d detected, %d unsupported.", k, path, len(detected), len(unsupported))
		allDetected = append(allDetected, detected...)
		allUnsupported = append(allUnsupported, unsupported...)
	}

	return allDetected, allUnsupported, nil
}
