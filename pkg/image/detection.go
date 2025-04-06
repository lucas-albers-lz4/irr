package image

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

// Error definitions for this package are centralized in errors.go

// Constants for image detection
const (
	// Patterns for detecting image references
	tagPattern       = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9]):(?P<tag>[a-zA-Z0-9][-a-zA-Z0-9._]+)$`
	digestPattern    = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9])@(?P<digest>sha256:[a-fA-F0-9]{64})$`
	defaultRegistry  = "docker.io"
	libraryNamespace = "library"
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
	Reference *ImageReference
	Path      []string
	Pattern   string      // "map", "string", "global"
	Original  interface{} // Original value (for template preservation)
}

// UnsupportedImage represents an unsupported image found during detection
type UnsupportedImage struct {
	Location []string
	Type     UnsupportedType
	Error    error
}

// UnsupportedType defines the type of unsupported image
type UnsupportedType int

const (
	UnsupportedTypeUnknown UnsupportedType = iota
	UnsupportedTypeMap
	UnsupportedTypeString
)

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
	PatternMap    = "map"    // Map-based image reference
	PatternString = "string" // Single string value (e.g., "nginx:latest")
	PatternGlobal = "global" // Global registry pattern
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
	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		r := regexp.MustCompile(pattern)
		regexps = append(regexps, r)
	}
	return regexps
}

// NewImageDetector creates a new ImageDetector instance
func NewImageDetector(ctx *DetectionContext) *ImageDetector {
	return &ImageDetector{context: ctx}
}

// DetectImages recursively traverses the values map to find image references
func (d *ImageDetector) DetectImages(values interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	debug.FunctionEnter("DetectImages")
	defer debug.FunctionExit("DetectImages")
	debug.DumpValue("Current Path", path)
	debug.DumpValue("Current Values", values)

	allDetected := make([]DetectedImage, 0)
	allUnsupported := make([]UnsupportedImage, 0)

	switch v := values.(type) {
	case map[string]interface{}:
		debug.Println("Processing map")

		// First, try to detect an image map at the current level
		if detectedImage, isImage, err := d.tryExtractImageFromMap(v, path); isImage {
			if err != nil {
				debug.Printf("Error extracting image from map at path %v: %v", path, err)
				allUnsupported = append(allUnsupported, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeMap,
					Error:    err,
				})
			} else if IsValidImageReference(detectedImage.Reference) {
				if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					debug.Printf("Detected map-based image at path %v: %v", path, detectedImage.Reference)
					allDetected = append(allDetected, *detectedImage)
				} else {
					debug.Printf("Skipping map-based image (not a source registry) at path %v: %v", path, detectedImage.Reference)
				}
			} else {
				debug.Printf("Skipping invalid map-based image reference at path %v: %v", path, detectedImage.Reference)
				// Optionally add to unsupported if strict mode requires full valid refs
			}
			// Don't recurse further if we identified this map as an image structure
			return allDetected, allUnsupported, nil
		}

		// If not an image map, recurse into its values
		for key, val := range v {
			newPath := append(append([]string{}, path...), key)
			detected, unsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				// Propagate errors, but maybe wrap them with path context?
				return nil, nil, fmt.Errorf("error processing path %v: %w", newPath, err)
			}
			allDetected = append(allDetected, detected...)
			allUnsupported = append(allUnsupported, unsupported...)
		}

	case []interface{}:
		debug.Println("Processing slice/array")
		// Only process arrays if the path suggests it might contain images
		// (e.g., spec.template.spec.containers)
		if d.context.Strict && !isImagePath(path) {
			debug.Printf("Skipping array processing at non-image path in strict mode: %v", path)
			return allDetected, allUnsupported, nil
		}
		for i, item := range v {
			indexPath := fmt.Sprintf("%s[%d]", strings.Join(path, "."), i)
			newPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i)) // Representation for path tracking
			debug.Printf("Recursing into array index %d at path %s", i, indexPath)
			detected, unsupported, err := d.DetectImages(item, newPath)
			if err != nil {
				return nil, nil, fmt.Errorf("error processing array path %v index %d: %w", path, i, err)
			}
			allDetected = append(allDetected, detected...)
			allUnsupported = append(allUnsupported, unsupported...)
		}

	case string:
		debug.Println("Processing string")
		// Check if path indicates this string *should* be an image
		if isImagePath(path) {
			detectedImage, err := d.tryExtractImageFromString(v, path)
			if err != nil {
				// Use errors.Is for checking sentinel errors
				isSpecificError := errors.Is(err, ErrInvalidImageString) || errors.Is(err, ErrEmptyImageReference) || errors.Is(err, ErrTemplateVariableInRepo)
				if !isSpecificError {
					// Report unexpected errors
					debug.Printf("Unexpected error parsing string image at path %v: %v", path, err)
					allUnsupported = append(allUnsupported, UnsupportedImage{
						Location: path,
						Type:     UnsupportedTypeString,
						Error:    err,
					})
				} else {
					// Log expected non-image string cases at debug level
					debug.Printf("String at image path %v is not a valid image reference (or is template): %s. Error: %v", path, v, err)
					if d.context.Strict && errors.Is(err, ErrInvalidImageString) {
						// In strict mode, invalid strings at image paths are unsupported
						allUnsupported = append(allUnsupported, UnsupportedImage{Location: path, Type: UnsupportedTypeString, Error: err})
					}
				}
			} else if detectedImage != nil && IsValidImageReference(detectedImage.Reference) {
				if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					debug.Printf("Detected string-based image at path %v: %v", path, detectedImage.Reference)
					allDetected = append(allDetected, *detectedImage)
				} else {
					debug.Printf("Skipping string-based image (not a source registry) at path %v: %v", path, detectedImage.Reference)
				}
			} else if detectedImage == nil {
				// String wasn't an image format (and no error returned from tryExtract)
				debug.Printf("String at path %v does not match image format (but no error returned): %s", path, v)
			}
		} else {
			debug.Printf("Skipping string at non-image path: %v", path)
			// Optionally, in very strict mode, try parsing anyway and warn/error if it LOOKS like an image?
		}

	case bool, float64, int, nil:
		// Ignore scalar types that cannot be images
		debug.Printf("Skipping non-string/map/slice type (%T) at path %v", v, path)
	default:
		// Handle unexpected types
		debug.Printf("Warning: Encountered unexpected type %T at path %v", v, path)
		// Depending on strictness, maybe add to unsupported
		// if d.context.Strict { allUnsupported = append(allUnsupported, UnsupportedImage{...}) }
	}

	return allDetected, allUnsupported, nil
}

// tryExtractImageFromMap attempts to parse an image reference from a map structure.
// It returns the DetectedImage, a boolean indicating if it was an image map, and an error.
func (d *ImageDetector) tryExtractImageFromMap(m map[string]interface{}, path []string) (*DetectedImage, bool, error) {
	ref := &ImageReference{Path: path}
	keys := make(map[string]bool)
	for k := range m {
		keys[k] = true
	}

	hasRepo := keys["repository"]
	hasTag := keys["tag"]
	hasRegistry := keys["registry"]
	hasDigest := keys["digest"]

	// Basic structural check - must have at least repository
	if !hasRepo {
		return nil, false, nil // Not an image map structure
	}

	// --- Extract Repository (Required) ---
	repoVal, ok := m["repository"]
	if !ok {
		return nil, true, ErrRepoNotFound // Should be caught by hasRepo, but defense-in-depth
	}
	repoStr, ok := repoVal.(string)
	if !ok {
		return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapRepo, repoVal) // Use canonical error
	}
	if repoStr == "" {
		return nil, true, fmt.Errorf("%w: repository cannot be empty", ErrInvalidImageMapRepo)
	}
	ref.Repository = repoStr

	// --- Extract Registry (Optional, check global context) ---
	if hasRegistry {
		regVal, _ := m["registry"]
		regStr, ok := regVal.(string)
		if !ok {
			return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapRegistryType, regVal) // Use canonical error
		}
		ref.Registry = regStr
	} else if d.context.GlobalRegistry != "" {
		debug.Printf("Using global registry '%s' for path %v", d.context.GlobalRegistry, path)
		ref.Registry = d.context.GlobalRegistry
	} else {
		ref.Registry = "" // Will be normalized later
	}

	// --- Extract Tag (Optional) ---
	if hasTag {
		tagVal, _ := m["tag"]
		tagStr, ok := tagVal.(string)
		if !ok {
			// Handle non-string tags gracefully if not strict template mode
			if d.context.TemplateMode {
				// Preserve non-string tags if they might be templates
				ref.Tag = fmt.Sprintf("%v", tagVal) // Store as string representation
				debug.Printf("Preserving potentially templated non-string tag at path %v: %v", path, ref.Tag)
			} else {
				return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapTagType, tagVal) // Use canonical error
			}
		} else {
			ref.Tag = tagStr
		}
	}

	// --- Extract Digest (Optional) ---
	if hasDigest {
		digestVal, _ := m["digest"]
		digestStr, ok := digestVal.(string)
		if !ok {
			return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapDigestType, digestVal) // Use canonical error
		}
		ref.Digest = digestStr
	}

	// --- Validation ---
	if ref.Tag != "" && ref.Digest != "" {
		return nil, true, ErrTagAndDigestPresent // Use canonical error
	}
	// In non-strict mode, allow missing tag/digest if we have a repo
	if ref.Tag == "" && ref.Digest == "" && !d.context.TemplateMode { // Allow empty in template mode
		debug.Printf("Warning: Image map at path %v missing tag and digest. Assuming 'latest'.", path)
		ref.Tag = "latest" // Or handle as error in strict mode? Current: default to latest
	}

	// Normalize
	NormalizeImageReference(ref)

	detected := &DetectedImage{
		Reference: ref,
		Path:      path,
		Pattern:   PatternMap,
		Original:  m, // Store original map for potential template preservation
	}

	return detected, true, nil
}

// tryExtractImageFromString attempts to parse an image reference from a string value.
func (d *ImageDetector) tryExtractImageFromString(imgStr string, path []string) (*DetectedImage, error) {
	debug.FunctionEnter("tryExtractImageFromString")
	defer debug.FunctionExit("tryExtractImageFromString")
	debug.Printf("Attempting to parse string at path %v: '%s'", path, imgStr)

	if imgStr == "" {
		debug.Println("Input string is empty.")
		return nil, ErrEmptyImageReference // Use canonical error
	}

	// Handle potential template variables
	if d.context.TemplateMode && strings.Contains(imgStr, "{{") {
		debug.Printf("Template detected in string '%s' at path %v. Skipping parsing, preserving original.", imgStr, path)
		// Heuristic: If it looks like repo:{{tag}}, treat repo as static?
		// For now, treat the whole thing as opaque if template detected.
		// We still need to decide *if* it's an image string based on path.
		// Let's assume if path matches, it IS an image, just templated.
		ref := &ImageReference{
			// Attempt a simple split for repo, but mark as potentially incomplete
			Repository: imgStr, // Store original templated string
			Tag:        "",     // Cannot reliably parse tag
			Digest:     "",
			Registry:   "", // Cannot reliably parse registry
			Path:       path,
		}
		// A very basic split attempt might inform normalization/source check
		parts := strings.SplitN(imgStr, ":", 2)
		if len(parts) > 0 {
			// Check if the first part contains template - if not, maybe use it?
			if !strings.Contains(parts[0], "{{") {
				// Potentially split registry/repo from the first part
				repoParts := strings.SplitN(parts[0], "/", 2)
				if len(repoParts) == 2 && strings.Contains(repoParts[0], ".") { // Looks like registry/repo
					ref.Registry = repoParts[0]
					ref.Repository = repoParts[1]
				} else {
					ref.Repository = parts[0] // Assume it's just repo
				}
			}
		}
		// Mark the reference somehow? Add a field `IsTemplated: true`?
		// For now, rely on Original field and potentially empty Tag/Digest.
		NormalizeImageReference(ref) // Normalize what we could parse
		return &DetectedImage{
			Reference: ref,
			Path:      path,
			Pattern:   PatternString,
			Original:  imgStr, // Preserve original templated string
		}, nil // Return nil error because we handled the template
	}

	// If not template mode or no template detected, parse normally
	ref, err := ParseImageReference(imgStr)
	if err != nil {
		debug.Printf("Failed to parse '%s' as image reference: %v", imgStr, err)
		// Return canonical errors for specific parsing failures
		if errors.Is(err, ErrInvalidImageRefFormat) || errors.Is(err, ErrInvalidRepoName) || errors.Is(err, ErrInvalidTagFormat) || errors.Is(err, ErrInvalidDigestFormat) {
			return nil, fmt.Errorf("%w: %w", ErrInvalidImageString, err) // Wrap original error
		}
		// Also check for empty string error, although checked earlier, belt-and-suspenders
		if errors.Is(err, ErrEmptyImageReference) {
			return nil, ErrEmptyImageReference // Return canonical error directly
		}
		return nil, err // Propagate other potential errors (e.g., from regex compilation if it failed)
	}

	ref.Path = path
	NormalizeImageReference(ref)

	debug.Printf("Successfully parsed string image: %+v", ref)
	return &DetectedImage{
		Reference: ref,
		Path:      path,
		Pattern:   PatternString,
		Original:  imgStr,
	}, nil
}

// IsValidImageReference performs basic validation on a parsed ImageReference
// Note: This checks structure, not necessarily registry reachability etc.
func IsValidImageReference(ref *ImageReference) bool {
	if ref == nil {
		return false
	}
	if ref.Repository == "" {
		return false
	}
	if ref.Tag == "" && ref.Digest == "" {
		// Allow if it might be a template tag
		// How to reliably detect this without full template engine?
		// For now, allow empty tag/digest if potentially templated.
		// A stricter validation might happen later.
		// return false // Stricter check
		return true // Looser check, assume templates are possible
	}
	if ref.Tag != "" && ref.Digest != "" {
		return false // Cannot have both
	}
	// Add more checks? e.g., valid chars in repo/tag?
	// if !isValidRepositoryName(ref.Repository) { return false }
	// if ref.Tag != "" && !isValidTag(ref.Tag) { return false }
	// if ref.Digest != "" && !isValidDigest(ref.Digest) { return false }
	return true
}

// ParseImageReference parses a standard Docker image reference string (e.g., registry/repo:tag or repo@digest)
// It returns an ImageReference or an error if parsing fails.
func ParseImageReference(imgStr string) (*ImageReference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s'", imgStr)

	if imgStr == "" {
		debug.Println("Error: Input string is empty.")
		return nil, ErrEmptyImageReference // Use canonical error
	}

	// Check for invalid tag + digest combination first
	digestIndexCheck := strings.Index(imgStr, "@sha256:")
	if digestIndexCheck > 0 {
		tagIndexCheck := strings.LastIndex(imgStr[:digestIndexCheck], ":")
		slashIndexCheck := strings.Index(imgStr, "/")
		if tagIndexCheck > 0 && (slashIndexCheck == -1 || tagIndexCheck > slashIndexCheck) {
			debug.Printf("Error: Found both tag separator ':' and digest separator '@' in '%s'", imgStr)
			return nil, ErrTagAndDigestPresent
		}
	}

	var ref ImageReference

	// --- Prioritize Digest Parsing ---
	if strings.Contains(imgStr, "@") {
		parts := strings.SplitN(imgStr, "@", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			debug.Printf("Invalid format for digest reference: '%s'", imgStr)
			return nil, ErrInvalidImageRefFormat
		}
		namePart := parts[0]
		digestPart := parts[1]

		// Validate digest part strictly
		if !isValidDigest(digestPart) {
			debug.Printf("Invalid digest format in '%s'", digestPart)
			return nil, ErrInvalidDigestFormat
		}
		ref.Digest = digestPart // Store the valid digest

		// Parse the name part (registry/repository)
		slashIndex := strings.Index(namePart, "/")
		if slashIndex == -1 {
			// Assumed to be repository only (e.g., "myimage@sha256:...")
			if !isValidRepositoryName(namePart) {
				debug.Printf("Invalid repository name in digest reference: '%s'", namePart)
				return nil, ErrInvalidRepoName
			}
			ref.Repository = namePart
			// Registry will be defaulted by NormalizeImageReference
		} else {
			// Potential registry/repository split
			potentialRegistry := namePart[:slashIndex]
			potentialRepo := namePart[slashIndex+1:]

			// Basic check: if first part contains '.' or ':', assume it's a registry
			if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
				if !isValidRegistryName(potentialRegistry) { // Optional stricter check?
					debug.Printf("Invalid registry name inferred: '%s'", potentialRegistry)
					// Treat as error or proceed assuming it's part of the repo? Let's be strict for now.
					// return nil, ErrInvalidRegistryName // Need this error defined
					return nil, fmt.Errorf("invalid registry name detected: %s", potentialRegistry)
				}
				if !isValidRepositoryName(potentialRepo) {
					debug.Printf("Invalid repository name after registry split: '%s'", potentialRepo)
					return nil, ErrInvalidRepoName
				}
				ref.Registry = potentialRegistry
				ref.Repository = potentialRepo
			} else {
				// Assume the whole namepart is the repository
				if !isValidRepositoryName(namePart) {
					debug.Printf("Invalid repository name (treated as whole): '%s'", namePart)
					return nil, ErrInvalidRepoName
				}
				ref.Repository = namePart
				// Registry will be defaulted by NormalizeImageReference
			}
		}

		// Make sure repository is not empty after parsing name part
		if ref.Repository == "" {
			debug.Println("Error: Missing repository in digest reference after parsing name.")
			return nil, ErrInvalidRepoName
		}

		NormalizeImageReference(&ref) // Normalize AFTER validation and parsing
		debug.Printf("Parsed digest ref: Registry='%s', Repo='%s', Digest='%s'", ref.Registry, ref.Repository, ref.Digest)
		return &ref, nil
	} // --- End Digest Parsing ---

	// --- Try Tag Parsing (only if no '@' found) ---
	tagMatches := tagRegexCompiled.FindStringSubmatch(imgStr)
	if len(tagMatches) > 0 {
		debug.Println("Tag pattern matched")
		matchMap := make(map[string]string)
		for i, name := range tagRegexCompiled.SubexpNames() {
			if i != 0 && name != "" {
				matchMap[name] = tagMatches[i]
			}
		}
		ref.Registry = matchMap["registry"]
		ref.Repository = matchMap["repository"]
		ref.Tag = matchMap["tag"]
		debug.Printf("Parsed tag ref: Registry='%s', Repo='%s', Tag='%s'", ref.Registry, ref.Repository, ref.Tag)

		// Validation
		if ref.Repository == "" {
			debug.Println("Error: Missing repository in tag reference.")
			return nil, ErrInvalidRepoName // Return nil ref on error
		}
		// Check tag validity *before* normalization
		if !isValidTag(ref.Tag) {
			debug.Println("Error: Invalid tag format.")
			return nil, ErrInvalidTagFormat // Return specific error
		}

		NormalizeImageReference(&ref) // Normalize AFTER validation
		return &ref, nil
	} // --- End Tag Parsing ---

	// --- Handle Repository-Only Case (fallback) ---
	if !isValidRepositoryName(imgStr) {
		// If it's not a valid repository name either, then the format is truly invalid
		debug.Printf("Error: String '%s' did not match tag or digest patterns and is not a valid repository name.", imgStr)
		return nil, ErrInvalidImageRefFormat
	}

	// Only proceed if it *is* a potentially valid repository name
	debug.Println("Assuming repository-only reference, defaulting tag to 'latest'")
	ref.Repository = imgStr
	// Tag and Digest are already empty
	NormalizeImageReference(&ref) // Will set default registry and latest tag
	return &ref, nil

	// This final return is now logically unreachable if the above check handles the failure case
	// return nil, ErrInvalidImageRefFormat
}

// Commented regex for tag validation (simplified)
// tagRegex = regexp.MustCompile(`^[\w][\w.-]{0,127}$`)

// isValidTag checks if a tag is valid (basic check)
func isValidTag(tag string) bool {
	// Basic length check
	return len(tag) > 0 && len(tag) <= 128
}

// Commented regex for digest validation
// digestRegex = regexp.MustCompile(`^[a-zA-Z0-9_+.-]+:[a-fA-F0-9]{32,}$`)
// Stricter sha256 check
var digestCharsRegex = regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`)

// isValidDigest checks if a digest is valid (basic check)
func isValidDigest(digest string) bool {
	return digestCharsRegex.MatchString(digest)
}

// isValidRegistryName checks if a registry name is plausible (basic check)
func isValidRegistryName(registry string) bool {
	if registry == "" {
		return true // Empty is allowed, defaults to docker.io
	}
	// Basic check: contains a dot or is localhost (common patterns)
	return strings.Contains(registry, ".") || strings.Contains(registry, ":") || registry == "localhost"
}

// isValidRepositoryName checks if a repository name is plausible
func isValidRepositoryName(repo string) bool {
	if repo == "" {
		return false
	}
	// Check for invalid consecutive slashes or colons
	if strings.Contains(repo, "//") || strings.Contains(repo, "::") || strings.Contains(repo, ":/") || strings.Contains(repo, "/:") {
		debug.Printf("Repository name '%s' contains invalid consecutive separators or colons.", repo)
		return false
	}
	// Check for invalid characters (allow lowercase alphanumeric, ., _, -, /)
	// A simple check: disallow anything NOT in that set, except we already checked for : and space
	// More precise regex might be needed for full compliance, but let's add a basic check for colons specifically.
	if strings.Contains(repo, ":") {
		debug.Printf("Repository name '%s' contains invalid character ':'.", repo)
		return false
	}

	// Original simplified check: not empty, doesn't start/end with /, doesn't contain space
	isValid := repo != "" && !strings.HasPrefix(repo, "/") && !strings.HasSuffix(repo, "/") && !strings.Contains(repo, " ")
	if !isValid {
		debug.Printf("Repository name '%s' failed basic checks (empty, starts/ends with /, contains space).", repo)
	}
	return isValid
}

// NormalizeImageReference ensures registry and potentially repository are set correctly,
// especially handling Docker Library images (e.g., "nginx" -> "docker.io/library/nginx")
func NormalizeImageReference(ref *ImageReference) {
	if ref == nil {
		return
	}

	// Default registry ONLY if none was parsed
	if ref.Registry == "" {
		ref.Registry = defaultRegistry // "docker.io"
		// Handle Docker Library images (prepend "library/") only when using the default registry
		if !strings.Contains(ref.Repository, "/") && !strings.HasPrefix(ref.Repository, libraryNamespace+"/") {
			ref.Repository = libraryNamespace + "/" + ref.Repository
			debug.Printf("Normalized Docker Library image: %s -> %s", ref.Repository, libraryNamespace+"/"+ref.Repository)
		}
	} else {
		// If registry was parsed, check if it normalizes to docker.io anyway (e.g., index.docker.io)
		// We don't want to overwrite an explicit registry like gcr.io with docker.io here.
		if NormalizeRegistry(ref.Registry) == defaultRegistry {
			ref.Registry = defaultRegistry // Ensure canonical docker.io if it is docker.io
		}
		// No need to prepend "library/" if an explicit registry was provided.
	}

	// Ensure tag is set ONLY if BOTH tag and digest are empty
	if ref.Tag == "" && ref.Digest == "" {
		ref.Tag = "latest"
	}
}

// isImagePath checks if the given path likely corresponds to an image field
func isImagePath(path []string) bool {
	pathStr := strings.Join(path, ".")

	// Check against known non-image patterns first
	for _, r := range nonImagePathRegexps {
		if r.MatchString(pathStr) {
			debug.Printf("Path '%s' matched non-image pattern: %s", pathStr, r.String())
			return false
		}
	}

	// Check against known image patterns
	for _, r := range imagePathRegexps {
		if r.MatchString(pathStr) {
			debug.Printf("Path '%s' matched image pattern: %s", pathStr, r.String())
			return true
		}
	}

	debug.Printf("Path '%s' did not match any known image or non-image patterns.", pathStr)
	// Default behavior if no pattern matches? Assume not an image unless explicitly matched?
	return false // Default to false if no specific image pattern matches
}

// Regex compilation moved here to avoid init cycles if defined globally with errors
var (
	digestRegexCompiled = regexp.MustCompile(digestPattern)
	tagRegexCompiled    = regexp.MustCompile(tagPattern)
)

// Helper function for backwards compatibility or simpler calls
// Deprecated: Use ImageDetector with context instead.
func DetectImages(values interface{}, path []string, sourceRegistries []string, excludeRegistries []string, strict bool) ([]DetectedImage, []UnsupportedImage, error) {
	ctx := &DetectionContext{
		SourceRegistries:  sourceRegistries,
		ExcludeRegistries: excludeRegistries,
		Strict:            strict,
		TemplateMode:      true, // Assume template mode for compatibility
	}
	detector := NewImageDetector(ctx)
	// Ensure this calls the METHOD on the detector instance
	return detector.DetectImages(values, path)
}
