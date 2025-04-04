package image

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lalbers/helm-image-override/pkg/debug"
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

	// Handle docker.io special cases
	if registry == "docker.io" || registry == "index.docker.io" {
		return defaultRegistry
	}

	return registry
}

// SanitizeRegistryForPath makes a registry name safe for use in a path
func SanitizeRegistryForPath(registry string) string {
	// Replace potentially problematic characters
	sanitized := strings.ReplaceAll(registry, ":", "-")
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
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
		"image$",
		"^.*\\.image$",
		"^.*\\.images\\[\\d+\\]$",
		"^spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$",
		"^spec\\.template\\.spec\\.initContainers\\[\\d+\\]\\.image$",
		"^spec\\.jobTemplate\\.spec\\.template\\.spec\\.containers\\[\\d+\\]\\.image$",
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
func (d *ImageDetector) DetectImages(values interface{}, path []string) ([]DetectedImage, error) {
	debug.FunctionEnter("DetectImages")
	defer debug.FunctionExit("DetectImages")

	var detected []DetectedImage

	// Handle nil values
	if values == nil {
		return detected, nil
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
		// Try to detect as an image map first
		if ref, pattern, err := d.tryExtractImageFromMap(v); err != nil {
			return nil, err // Propagate type errors
		} else if ref != nil {
			detected = append(detected, DetectedImage{
				Location:     path,
				LocationType: TypeMapRegistryRepositoryTag,
				Reference:    ref,
				Pattern:      pattern,
				Original:     v,
			})
			return detected, nil
		}

		// If not an image map, process each key
		for k, val := range v {
			newPath := append(path, k)

			// Skip processing global registry here since we handled it above
			if len(path) == 0 && k == "global" {
				continue
			}

			// Recurse into value
			subDetected, err := d.DetectImages(val, newPath)
			if err != nil {
				return nil, fmt.Errorf("error processing key %s: %w", k, err)
			}
			detected = append(detected, subDetected...)
		}

	case []interface{}:
		for i, val := range v {
			newPath := append(path, fmt.Sprintf("[%d]", i))
			subDetected, err := d.DetectImages(val, newPath)
			if err != nil {
				return nil, fmt.Errorf("error processing array index %d: %w", i, err)
			}
			detected = append(detected, subDetected...)
		}

	case string:
		if isImagePath(path) || isStrictImageString(v) {
			if ref, err := tryExtractImageFromString(v); err == nil && ref != nil {
				// Apply global registry if needed
				if ref.Registry == "" && d.context.GlobalRegistry != "" {
					ref.Registry = d.context.GlobalRegistry
				}
				detected = append(detected, DetectedImage{
					Location:     path,
					LocationType: TypeRepositoryTag,
					Reference:    ref,
					Pattern:      "string",
					Original:     v,
				})
			}
		}
	}

	return detected, nil
}

// tryExtractImageFromMap attempts to extract an image reference from a map
func (d *ImageDetector) tryExtractImageFromMap(m map[string]interface{}) (*ImageReference, string, error) {
	repository, hasRepository := m["repository"]
	tag, hasTag := m["tag"]
	registry, hasRegistry := m["registry"]

	if !hasRepository {
		return nil, "", nil
	}

	repoStr, ok := repository.(string)
	if !ok {
		return nil, "", fmt.Errorf("repository is not a string")
	}

	ref := &ImageReference{Repository: repoStr}

	// Handle registry with precedence:
	// 1. Map-specific registry
	// 2. Global registry from context
	// 3. Default registry (docker.io)
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

// isGlobalRegistry checks if a key/value pair represents a global registry setting
func (d *ImageDetector) isGlobalRegistry(key string, value interface{}) bool {
	return (strings.HasPrefix(key, "global.") && strings.Contains(key, "registry")) ||
		(key == "registry" && strings.Contains(strings.ToLower(key), "global"))
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
		// Validate digest format
		if !strings.HasPrefix(ref.Digest, "sha256:") || len(ref.Digest) != 71 {
			return nil, fmt.Errorf("invalid digest format")
		}
	}

	// Split remaining part by : to handle tag
	tagParts := strings.SplitN(parts[0], ":", 2)
	if len(tagParts) == 2 {
		ref.Tag = tagParts[1]
		// Validate tag format
		if !isValidTag(ref.Tag) {
			return nil, fmt.Errorf("invalid tag format")
		}
	}

	// Handle repository part
	repoStr := tagParts[0]
	if strings.Contains(repoStr, "/") {
		// Has registry or organization
		repoParts := strings.SplitN(repoStr, "/", 2)
		if strings.Contains(repoParts[0], ".") || strings.Contains(repoParts[0], ":") {
			// First part contains . or : indicating it's a registry
			ref.Registry = repoParts[0]
			ref.Repository = repoParts[1]
		} else {
			// No registry specified, use default
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

	// Additional validation to ensure this looks like an image reference
	if !isValidImageReference(ref) {
		return nil, fmt.Errorf("invalid image reference format")
	}

	// Only add library prefix for docker.io registry and single-component repository names
	if ref.Registry == defaultRegistry && !strings.Contains(ref.Repository, "/") {
		ref.Repository = fmt.Sprintf("%s/%s", libraryNamespace, ref.Repository)
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
	ref.Registry = NormalizeRegistry(ref.Registry)

	// Handle Docker library images
	if !strings.Contains(ref.Repository, "/") && isValidDockerLibraryName(ref.Repository) {
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
		return false
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

	// Check tag or digest
	if ref.Tag != "" && ref.Digest != "" {
		return false // Can't have both
	}
	if ref.Tag == "" && ref.Digest == "" {
		return false // Must have one
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
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !isValidRepositoryPart(part) {
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
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Package image provides functionality for detecting and manipulating container image references.
// ... existing code ...
