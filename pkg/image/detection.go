// Package image provides functionality for detecting, parsing, and manipulating container image references within Helm values.
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
	UnsupportedTypeNonSource
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

// SanitizeRegistryForPath makes a registry name safe for use in a path component.
// It primarily removes dots and ports.
func SanitizeRegistryForPath(registry string) string {
	// Normalize docker.io variants first
	if registry == "docker.io" || registry == "index.docker.io" || registry == "" {
		return "dockerio"
	}

	// Strip port number if present
	if portIndex := strings.LastIndex(registry, ":"); portIndex != -1 {
		potentialPort := registry[portIndex+1:]
		if _, err := fmt.Sscan(potentialPort, new(int)); err == nil {
			registry = registry[:portIndex]
		} else {
			debug.Printf("SanitizeRegistryForPath: ':' found in '%s' but part after it ('%s') is not numeric, not treating as port.", registry, potentialPort)
		}
	}

	// Remove dots
	sanitized := strings.ReplaceAll(registry, ".", "")

	// DO NOT replace slashes

	// DO NOT add port back

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
		// Always process arrays, path check should happen for items within if needed
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
		processThisString := false
		isKnownImagePath := isImagePath(path)
		looksLikeAnImage := strings.Contains(v, ":") || strings.Contains(v, "@") || strings.Count(v, "/") > 0

		if d.context.Strict {
			// In strict mode, process if it's a known image path OR if it looks like an image (even if path is unknown)
			// We will categorize errors/non-source later.
			processThisString = isKnownImagePath || looksLikeAnImage
			if !processThisString && !isNonImagePath(path) { // Log only truly ambiguous skips
				debug.Printf("Strict mode: Skipping string at path %v (not known image path, doesn't look like image): '%s'", path, v)
			} else if !processThisString && isNonImagePath(path) {
				debug.Printf("Strict mode: Skipping string at known non-image path %v: '%s'", path, v)
			}
		} else {
			// Non-strict: Process if known path OR looks like image, AND not known non-image path
			processThisString = (isKnownImagePath || looksLikeAnImage) && !isNonImagePath(path)
			if !processThisString {
				debug.Printf("Non-strict: Skipping string at path %v (isKnown=%v, looksLike=%v, isNonImage=%v): '%s'", path, isKnownImagePath, looksLikeAnImage, isNonImagePath(path), v)
			}
		}

		if processThisString {
			debug.Printf("Processing string at path %v: '%s'", path, v)
			detectedImage, err := d.tryExtractImageFromString(v, path)

			if err == nil && detectedImage != nil && IsValidImageReference(detectedImage.Reference) {
				// Parsed OK, valid reference
				debug.Printf("Checking IsSourceRegistry for path %v, image %s", path, detectedImage.Reference.String())
				isSource := IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries)
				debug.Printf("IsSourceRegistry result for path %v: %v", path, isSource)

				if isSource {
					// Valid source image
					if !d.context.Strict {
						// Non-strict: Add to detected
						debug.Printf("Detected valid source string-based image at path %v: %v", path, detectedImage.Reference)
						allDetected = append(allDetected, *detectedImage)
					} else { // Strict mode
						if isKnownImagePath {
							// Strict & Known Path: Add to detected
							debug.Printf("Detected valid source string-based image at known path %v: %v", path, detectedImage.Reference)
							allDetected = append(allDetected, *detectedImage)
						} else {
							// Strict & Ambiguous Path: Mark unsupported
							debug.Printf("Strict mode: Parsed valid source image '%s' at ambiguous path %v. Marking unsupported.", v, path)
							allUnsupported = append(allUnsupported, UnsupportedImage{Location: path, Type: UnsupportedTypeString, Error: ErrAmbiguousStringPath})
						}
					}
				} else { // Not a source registry
					debug.Printf("Entered 'Not a source registry' block for path %v", path)
					debug.Printf("String parsed OK but not a source registry at path %v: %v", path, detectedImage.Reference)
					if d.context.Strict {
						// Strict: Mark unsupported (NonSource)
						debug.Printf("Strict mode: Marking non-source image as unsupported: %v", path)
						allUnsupported = append(allUnsupported, UnsupportedImage{Location: path, Type: UnsupportedTypeNonSource, Error: nil})
					}
					// Non-strict ignores non-source images
				}
			} else { // Parsing failed OR reference is invalid
				parseErr := err
				if parseErr == nil {
					parseErr = fmt.Errorf("parsed reference is invalid")
					debug.Printf("String parsed but reference invalid at path %v: '%s'", path, v)
				} else {
					debug.Printf("String parsing failed at path %v for value '%s': %v", path, v, parseErr)
				}

				if d.context.Strict {
					// Strict: Any parse failure/invalid ref for a processed string is unsupported
					finalError := parseErr
					// Check if the failure was ambiguous (looks like image, but not known path AND parse failed/invalid)
					if !isKnownImagePath && looksLikeAnImage {
						debug.Printf("Strict mode: Marking ambiguous/unparseable string as unsupported: %v (original error: %v)", path, parseErr)
						// Wrap the original parse error with the ambiguous path error if it's not already that error
						if !errors.Is(parseErr, ErrAmbiguousStringPath) {
							finalError = fmt.Errorf("%w: %w", ErrAmbiguousStringPath, parseErr)
						}
					} else {
						debug.Printf("Strict mode: Marking invalid/unparseable string at known path as unsupported: %v", path)
					}
					allUnsupported = append(allUnsupported, UnsupportedImage{Location: path, Type: UnsupportedTypeString, Error: finalError})
				}
				// Non-strict ignores parse failures
			}
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

// equalPath checks if two path slices are identical.
func equalPath(p1, p2 []string) bool {
	if len(p1) != len(p2) {
		return false
	}
	for i := range p1 {
		if p1[i] != p2[i] {
			return false
		}
	}
	return true
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
	registrySource := "none"
	if hasRegistry {
		regVal := m["registry"]
		if registryExists := (regVal != nil); registryExists {
			if regStr, regIsString := regVal.(string); regIsString {
				ref.Registry = regStr
				registrySource = "map"
			} else {
				return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapRegistryType, regVal)
			}
		}
		// If regVal was nil, fall through to check global context
	}

	// If registry wasn't set from the map, check global context
	if registrySource == "none" {
		if d.context.GlobalRegistry != "" {
			ref.Registry = d.context.GlobalRegistry
			registrySource = "global"
			debug.Printf("Using global registry '%s' for path %v", ref.Registry, path)
		} else {
			ref.Registry = "" // Explicitly empty, normalization will handle default
			registrySource = "default (pending normalization)"
		}
	}
	debug.Printf("Registry source for path %v: %s (Value: '%s')", path, registrySource, ref.Registry)

	// --- Extract Tag (Optional) ---
	if hasTag {
		tagVal := m["tag"]
		if tagIsString, ok := tagVal.(string); ok {
			ref.Tag = tagIsString
		} else {
			// Handle non-string tags gracefully if not strict template mode
			if d.context.TemplateMode {
				// Preserve non-string tags if they might be templates
				ref.Tag = fmt.Sprintf("%v", tagVal) // Store as string representation
				debug.Printf("Preserving potentially templated non-string tag at path %v: %v", path, ref.Tag)
			} else {
				return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapTagType, tagVal) // Use canonical error
			}
		}
	}

	// --- Extract Digest (Optional) ---
	if hasDigest {
		digestVal := m["digest"]
		if digestIsString, ok := digestVal.(string); ok {
			ref.Digest = digestIsString
		} else {
			return nil, true, fmt.Errorf("%w: found type %T", ErrInvalidImageMapDigestType, digestVal) // Use canonical error
		}
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
		// Check repository name validity *after* regex match but *before* normalization
		if !isValidRepositoryName(ref.Repository) {
			debug.Printf("Error: Invalid repository name format parsed: '%s'", ref.Repository)
			return nil, ErrInvalidRepoName
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

	// Regex for allowed characters in repository components
	// (lowercase alphanumeric + separators ., _, -)
	allowedChars := `^[a-z0-9]+(?:[._-][a-z0-9]+)*$`
	allowedCharsRegex := regexp.MustCompile(allowedChars)

	// Check overall length
	if len(repo) == 0 || len(repo) > 255 {
		debug.Printf("[DEBUG isValidRepositoryName] Repository name '%s' has invalid length %d.", repo, len(repo))
		return false
	}

	// Split into components and validate each one
	components := strings.Split(repo, "/")
	for _, component := range components {
		if !allowedCharsRegex.MatchString(component) {
			debug.Printf("[DEBUG isValidRepositoryName] Repository component '%s' in '%s' contains invalid characters.", component, repo)
			return false
		}
	}

	// Check for invalid consecutive slashes or colons (already done above, keep for safety)
	if strings.Contains(repo, "//") || strings.Contains(repo, "::") || strings.Contains(repo, ":/") || strings.Contains(repo, "/:") {
		debug.Printf("[DEBUG isValidRepositoryName] Repository name '%s' contains invalid consecutive separators.", repo)
		return false
	}

	// Ensure it doesn't contain colons (tags/digests handled separately)
	if strings.Contains(repo, ":") {
		debug.Printf("[DEBUG isValidRepositoryName] Repository name '%s' contains invalid character ':'.", repo)
		return false
	}

	// Check for uppercase letters (redundant with regex, but keep for clarity for now)
	if repo != strings.ToLower(repo) {
		debug.Printf("[DEBUG isValidRepositoryName] Repository '%s' contains uppercase letters. Returning false.", repo)
		return false
	}

	// Simplified basic checks (redundant with regex, but keep for safety)
	isValid := !strings.HasPrefix(repo, "/") && !strings.HasSuffix(repo, "/") && !strings.Contains(repo, " ")
	if !isValid {
		debug.Printf("[DEBUG isValidRepositoryName] Repository '%s' failed basic checks (starts/ends with /, contains space). Returning false.", repo)
		return false // Return false if basic checks fail
	}

	// If all checks pass
	debug.Printf("[DEBUG isValidRepositoryName] Repository '%s' passed all checks. Returning true.", repo)
	return true // Return true only if all checks pass
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
			debug.Printf("Normalized Docker Library image (default registry): %s -> %s", ref.Repository, libraryNamespace+"/"+ref.Repository)
		}
	} else {
		// If registry was explicitly provided (either from map or global context)
		normalizedReg := NormalizeRegistry(ref.Registry)
		if normalizedReg == defaultRegistry {
			ref.Registry = defaultRegistry // Ensure canonical docker.io
			// Handle library/ prefix ONLY if the original registry was effectively docker.io
			if !strings.Contains(ref.Repository, "/") && !strings.HasPrefix(ref.Repository, libraryNamespace+"/") {
				ref.Repository = libraryNamespace + "/" + ref.Repository
				debug.Printf("Normalized Docker Library image (explicit docker registry): %s -> %s", ref.Repository, libraryNamespace+"/"+ref.Repository)
			}
		} else {
			// Explicit registry is NOT docker.io, DO NOT prepend library/
			debug.Printf("Explicit non-docker registry '%s', not prepending library/ to '%s'", ref.Registry, ref.Repository)
		}
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

// isNonImagePath checks if the given path matches any known non-image patterns.
func isNonImagePath(path []string) bool {
	pathStr := strings.Join(path, ".")
	for _, r := range nonImagePathRegexps {
		if r.MatchString(pathStr) {
			return true
		}
	}
	return false
}

// Regex compilation moved here to avoid init cycles if defined globally with errors
var (
	tagRegexCompiled = regexp.MustCompile(tagPattern)
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
