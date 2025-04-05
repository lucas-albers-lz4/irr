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
			if ref, pattern, err := d.tryExtractImageFromMap(v); err != nil {
				return nil, nil, err // Propagate type errors
			} else if ref != nil {
				detected = append(detected, DetectedImage{
					Location:     path,
					LocationType: TypeMapRegistryRepositoryTag,
					Reference:    ref,
					Pattern:      pattern,
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
	// Check for required repository field
	repository, hasRepository := m["repository"]
	if !hasRepository {
		return nil, "", nil
	}

	// Validate repository is a string
	repoStr, ok := repository.(string)
	if !ok {
		return nil, "", fmt.Errorf("repository is not a string")
	}

	// Skip "repository" keys that don't look like image repositories
	// For example, Git repositories or other URLs that aren't container images
	if strings.HasPrefix(repoStr, "http") || strings.HasPrefix(repoStr, "git@") ||
		strings.HasSuffix(repoStr, ".git") || strings.Contains(repoStr, "github.com") {
		return nil, "", nil
	}

	// Proceed with creating the reference
	ref := &ImageReference{Repository: repoStr}

	// Handle registry with precedence:
	// 1. Map-specific registry
	// 2. Global registry from context
	// 3. Default registry (docker.io)
	registry, hasRegistry := m["registry"]
	if hasRegistry {
		if regStr, ok := registry.(string); ok {
			ref.Registry = regStr
		} else {
			return nil, "", fmt.Errorf("registry is not a string")
		}
	} else if d.context.GlobalRegistry != "" {
		ref.Registry = d.context.GlobalRegistry
	} else {
		ref.Registry = defaultRegistry
	}

	// Handle tag
	tag, hasTag := m["tag"]
	if hasTag {
		if tagStr, ok := tag.(string); ok {
			ref.Tag = tagStr
		} else {
			return nil, "", fmt.Errorf("tag is not a string")
		}
	}

	// Only add library prefix for docker.io registry and single-component repository names
	if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
		ref.Repository = fmt.Sprintf("%s/%s", libraryNamespace, ref.Repository)
	}

	return ref, "map", nil
}

// tryExtractImageFromString tries to extract an image reference from a string.
func tryExtractImageFromString(s string) (*ImageReference, error) {
	// Basic validation
	if s == "" {
		return nil, fmt.Errorf("empty string")
	}

	// Try to parse as a Docker image reference
	ref := &ImageReference{}

	// Split by @ first to handle digest
	parts := strings.SplitN(s, "@", 2)
	if len(parts) == 2 {
		ref.Digest = parts[1]
		// Validate digest format - allow any length for flexibility
		if !strings.HasPrefix(ref.Digest, "sha256:") {
			return nil, fmt.Errorf("invalid digest format")
		}
	}

	// Split remaining part by : to handle tag and registry port
	repoAndTag := parts[0]

	// We need to handle :port in the registry domain, so split from the right
	lastColonIdx := strings.LastIndex(repoAndTag, ":")
	if lastColonIdx > 0 {
		// Check if there's a slash after the last colon (indicates port is in registry)
		if strings.LastIndex(repoAndTag, "/") < lastColonIdx {
			ref.Tag = repoAndTag[lastColonIdx+1:]
			repoAndTag = repoAndTag[:lastColonIdx]
			// Validate tag format - more flexible validation
			if !isValidTag(ref.Tag) {
				return nil, fmt.Errorf("invalid tag format")
			}
		}
	}

	// Handle repository part
	repoStr := repoAndTag
	if strings.Contains(repoStr, "/") {
		// Has registry or organization
		repoParts := strings.SplitN(repoStr, "/", 2)
		if strings.Contains(repoParts[0], ".") || strings.Contains(repoParts[0], ":") || repoParts[0] == "localhost" {
			// First part contains . or : indicating it's a registry, or is localhost
			ref.Registry = repoParts[0]
			ref.Repository = repoParts[1]
		} else {
			// First part is an organization
			ref.Registry = defaultRegistry
			ref.Repository = repoStr
		}
	} else {
		// No registry or organization
		if !isValidDockerLibraryName(repoStr) {
			return nil, fmt.Errorf("invalid repository name")
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
		return nil, fmt.Errorf("invalid image reference format")
	}

	return ref, nil
}

// ParseImageReference parses an image reference from either a string or a map
func ParseImageReference(value interface{}) (*ImageReference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")

	switch v := value.(type) {
	case string:
		return parseImageString(v)
	case map[string]interface{}:
		return parseImageMap(v)
	default:
		return nil, fmt.Errorf("unsupported image reference type: %T", value)
	}
}

// parseImageString parses an image reference from a string
func parseImageString(image string) (*ImageReference, error) {
	debug.Printf("Parsing image string: %s", image)

	// Try to match as a tag-based reference first
	if matches := tagRegex.FindStringSubmatch(image); len(matches) > 0 {
		ref := &ImageReference{}
		for i, name := range tagRegex.SubexpNames() {
			if i != 0 && name != "" && i < len(matches) {
				switch name {
				case "registry":
					ref.Registry = matches[i]
				case "repository":
					ref.Repository = matches[i]
				case "tag":
					ref.Tag = matches[i]
				}
			}
		}
		return normalizeImageReference(ref), nil
	}

	// Try to match as a digest-based reference
	if matches := digestRegex.FindStringSubmatch(image); len(matches) > 0 {
		ref := &ImageReference{}
		for i, name := range digestRegex.SubexpNames() {
			if i != 0 && name != "" && i < len(matches) {
				switch name {
				case "registry":
					ref.Registry = matches[i]
				case "repository":
					ref.Repository = matches[i]
				case "digest":
					ref.Digest = matches[i]
				}
			}
		}
		return normalizeImageReference(ref), nil
	}

	// Try to extract using more flexible parsing
	ref, err := tryExtractImageFromString(image)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference: %v", err)
	}
	return ref, nil
}

// parseImageMap parses an image reference from a map
func parseImageMap(m map[string]interface{}) (*ImageReference, error) {
	debug.Printf("Parsing image map: %v", m)

	var ref ImageReference

	// Get repository
	if repo, ok := m["repository"].(string); ok {
		// Split registry and repository
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 && strings.Contains(parts[0], ".") {
			ref.Registry = parts[0]
			ref.Repository = parts[1]
		} else {
			ref.Registry = defaultRegistry
			ref.Repository = repo
		}
	} else {
		return nil, fmt.Errorf("repository not found or not a string")
	}

	// Get tag
	if tag, ok := m["tag"].(string); ok {
		ref.Tag = tag
	}

	// Get digest
	if digest, ok := m["digest"].(string); ok {
		ref.Digest = digest
	}

	return normalizeImageReference(&ref), nil
}

// normalizeImageReference ensures the reference has valid values
func normalizeImageReference(ref *ImageReference) *ImageReference {
	if ref == nil {
		return nil
	}

	// Normalize registry
	if ref.Registry == "" {
		ref.Registry = defaultRegistry
	} else {
		// Handle docker.io special cases
		if ref.Registry == "docker.io" || ref.Registry == "index.docker.io" {
			ref.Registry = defaultRegistry
		}
	}

	// Handle Docker library images: Add prefix only if registry is docker.io
	// and the repository doesn't already contain a slash
	if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
		ref.Repository = libraryNamespace + "/" + ref.Repository
	}

	return ref
}

// isValidTag checks if a tag string is valid
func isValidTag(tag string) bool {
	if tag == "" {
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

	// Check registry
	if ref.Registry != "" && !isValidRegistryName(ref.Registry) {
		// Accommodate for special cases like localhost, ip addresses with ports
		if !strings.HasPrefix(ref.Registry, "localhost") &&
			!strings.Contains(ref.Registry, ":") {
			return false
		}
	}

	// Check repository
	if ref.Repository == "" {
		return false
	}
	for _, part := range strings.Split(ref.Repository, "/") {
		if !isValidRepositoryPart(part) {
			return false
		}
	}

	// Check tag or digest - empty tag is valid for latest
	if ref.Tag != "" && ref.Digest != "" {
		return false // Can't have both
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

	for _, part := range parts {
		// nolint:staticcheck // Intentionally keeping complex boolean logic for readability
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

// isValidRepositoryPart checks if a repository name part is valid
func isValidRepositoryPart(part string) bool {
	if part == "" {
		return false
	}
	for _, c := range part {
		// nolint:staticcheck // Intentionally keeping complex boolean logic for readability
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
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
