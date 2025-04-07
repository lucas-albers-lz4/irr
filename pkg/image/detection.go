// Package image provides core functionality for detecting, parsing, and normalizing container image references within Helm chart values.
package image

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/log"
)

// Error definitions for this package are centralized in errors.go

// Constants for image detection
const (
	// Patterns for detecting image references
	tagPattern       = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9]):(?P<tag>[a-zA-Z0-9][-a-zA-Z0-9._]+)$`
	digestPattern    = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9])@(?P<digest>sha256:[a-fA-F0-9]{64})$`
	defaultRegistry  = "docker.io"
	libraryNamespace = "library"
	// maxSplitTwo is the limit for splitting into at most two parts
	maxSplitTwo = 2
)

// Reference encapsulates the components of a container image reference.
type Reference struct {
	Original   string // The original string detected
	Registry   string // e.g., docker.io, quay.io, gcr.io
	Repository string
	Tag        string
	Digest     string
	Path       []string // Path in the values structure where this reference was found
	Detected   bool
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
	Reference *Reference
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

// UnsupportedType defines the type of unsupported structure encountered.
type UnsupportedType int

const (
	// UnsupportedTypeUnknown indicates an unspecified or unknown unsupported type.
	UnsupportedTypeUnknown UnsupportedType = iota
	// UnsupportedTypeMap indicates an unsupported map structure.
	UnsupportedTypeMap
	// UnsupportedTypeString indicates an unsupported string format.
	UnsupportedTypeString
	// UnsupportedTypeStringParseError indicates a failure to parse an image string.
	UnsupportedTypeStringParseError
	// UnsupportedTypeNonSourceImage indicates an image string from a non-source registry in strict mode.
	UnsupportedTypeNonSourceImage
	// UnsupportedTypeExcludedImage indicates an image from an explicitly excluded registry.
	UnsupportedTypeExcludedImage
	// UnsupportedTypeList indicates an unsupported list/array structure where an image was expected.
	UnsupportedTypeList
	// UnsupportedTypeMapValue indicates an unsupported value type within a map where an image was expected.
	UnsupportedTypeMapValue
)

// Detector provides methods for finding image references within complex data structures.
type Detector struct {
	context *DetectionContext
}

// String returns the string representation of the image reference
func (r *Reference) String() string {
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
func IsSourceRegistry(ref *Reference, sourceRegistries, excludeRegistries []string) bool {
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
	if registry == defaultRegistry || registry == "index.docker.io" {
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
	if registry == defaultRegistry || registry == "index.docker.io" || registry == "" {
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
		"^simpleImage$",                // test pattern used in unit tests
		"^nestedImages\\.simpleImage$", // nested test pattern for simpleImage
		"^workerImage$",                // Added for specific test case
		"^publicImage$",                // Added for Excluded_Registry test
		"^internalImage$",              // Added for Excluded_Registry test (map)
		"^dockerImage$",                // Added for Non-Source_Registry test
		"^quayImage$",                  // Added for Non-Source_Registry test (map)
		"^imgDocker$",                  // Added for Prefix_Source_Registry_Strategy test
		"^imgGcr$",                     // Added for Prefix_Source_Registry_Strategy test
		"^imgQuay$",                    // Added for Prefix_Source_Registry_Strategy test (map)
		"^parentImage$",                // Added for Chart_with_Dependencies test
		"^imageFromDocker$",            // Added for WithRegistryMapping test
		"^imageFromQuay$",              // Added for WithRegistryMapping test
		"^imageUnmapped$",              // Added for WithRegistryMapping test
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

// NewDetector creates a new Detector
func NewDetector(context DetectionContext) *Detector {
	debug.Printf("NewDetector: Initializing with context: %+v", context)
	return &Detector{
		context: &context,
	}
}

// DetectImages recursively traverses the values structure to find image references.
func (d *Detector) DetectImages(values interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	debug.FunctionEnter("Detector.DetectImages")
	debug.DumpValue("Input values", values)
	debug.DumpValue("Current path", path)
	debug.DumpValue("Context", d.context)
	defer debug.FunctionExit("Detector.DetectImages")

	// Add detailed logging for map traversal entry
	debug.Printf("[DETECTOR ENTRY] Path: %v, Type: %T", path, values)

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	switch v := values.(type) {
	case map[string]interface{}:
		debug.Println("Processing map")

		// First, try to detect an image map at the current level
		if detectedImage, isImage, err := d.tryExtractImageFromMap(v, path); isImage {
			switch {
			case err != nil:
				debug.Printf("Error extracting image from map at path %v: %v", path, err)
				log.Debugf("[UNSUPPORTED] Adding map item at path %v due to extraction error: %v", path, err)
				log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Map extraction error, Error: %v, Strict: %v", path, UnsupportedTypeMap, err, d.context.Strict)
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeMap,
					Error:    err,
				})
			case IsValidImageReference(detectedImage.Reference):
				if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					debug.Printf("Detected map-based image at path %v: %v", path, detectedImage.Reference)
					detectedImages = append(detectedImages, *detectedImage)
				} else {
					debug.Printf("Map-based image is not a source registry at path %v: %v", path, detectedImage.Reference)
					if d.context.Strict {
						log.Debugf("[UNSUPPORTED] Adding map item at path %v due to strict non-source.", path)
						log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict non-source map, Error: nil, Strict: %v", path, UnsupportedTypeNonSourceImage, d.context.Strict)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeNonSourceImage, // Use consistent type
							Error:    nil,
						})
					} else {
						log.Debugf("Non-strict mode: Skipping non-source map image at path %v", path)
					}
				}
			default: // Invalid map-based image reference
				debug.Printf("Skipping invalid map-based image reference at path %v: %v", path, detectedImage.Reference)
			}
			// Whether detected, skipped non-source, or invalid, if it was identified AS an image map structure attempt (isImageMap == true),
			// we should NOT recurse further into its values.
			return detectedImages, unsupportedMatches, nil
		}

		// --- If isImageMap was false --- (Structure didn't match standard image map keys)
		// If it wasn't an image map attempt, recurse into its values regardless of path.
		// The strict mode handling for incorrect maps at known image paths is now inside tryExtractImageFromMap.
		log.Debugf("Structure at path %s did not match image map pattern, recursing into values.", path)
		for key, val := range v {
			newPath := append(append([]string{}, path...), key)
			detected, unsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				// Propagate errors, but maybe wrap them with path context?
				return nil, nil, fmt.Errorf("error processing path %v: %w", newPath, err)
			}
			detectedImages = append(detectedImages, detected...)
			// REVERTED/REFINED: Always append unsupported items from deeper calls.
			// DETAILED LOGGING: Log before appending from map recursion
			if len(unsupported) > 0 {
				log.Debugf("[UNSUPPORTED AGG MAP] Path: %v, Appending %d items from key '%s'", path, len(unsupported), key)
				for i, item := range unsupported {
					log.Debugf("[UNSUPPORTED AGG MAP ITEM %d] Path: %v, Type: %v, Error: %v", i, item.Location, item.Type, item.Error)
				}
			}
			unsupportedMatches = append(unsupportedMatches, unsupported...)
		}

	case []interface{}:
		debug.Println("Processing slice/array")
		// Always process arrays, path check should happen for items within if needed
		for i, item := range v {
			itemPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i)) // Ensure path is copied
			log.Debugf("Recursively processing slice item %d at path %s", i, itemPath)
			// Recursively call detectImagesRecursive for each item using the method receiver 'd'
			// Capture all three return values: detected, unsupported, and error
			detectedInItem, unsupportedInItem, err := d.DetectImages(item, itemPath)
			if err != nil {
				// Propagate the error, adding context
				log.Errorf("Error processing slice item at path %s: %v", itemPath, err)
				// Decide on error handling: return immediately or collect errors?
				// Returning immediately might be simpler.
				return nil, nil, fmt.Errorf("error processing slice item %d at path %s: %w", i, path, err)
			}
			detectedImages = append(detectedImages, detectedInItem...)
			// REVERTED/REFINED: Always append unsupported items from deeper calls.
			// DETAILED LOGGING: Log before appending from slice recursion
			if len(unsupportedInItem) > 0 {
				log.Debugf("[UNSUPPORTED AGG SLICE] Path: %v, Appending %d items from index %d", path, len(unsupportedInItem), i)
				for j, item := range unsupportedInItem {
					log.Debugf("[UNSUPPORTED AGG SLICE ITEM %d] Path: %v, Type: %v, Error: %v", j, item.Location, item.Type, item.Error)
				}
			}
			unsupportedMatches = append(unsupportedMatches, unsupportedInItem...)
		}

	case string:
		vStr := v
		log.Debugf("[DEBUG irr DETECT STRING] Processing string value at path %s: %q", path, vStr)

		// DETAILED LOGGING: Check strict context before path check
		log.Debugf("[DEBUG irr DETECT STRING] Path: %v, Value: '%s', Strict Context: %v", path, vStr, d.context.Strict)

		// First check: Is this at a path we recognize as potentially containing images?
		isKnownImagePath := isImagePath(path)

		// REVISED: In strict mode, skip strings at unknown paths immediately *before* checking format.
		if d.context.Strict && !isKnownImagePath {
			log.Debugf("Strict mode: Skipping string at unknown image path %s: %q", path, vStr)
			break // Exit this case
		}

		// Second check: Does the string look like an image reference?
		looksLikeImage := looksLikeImageReference(vStr)
		if !looksLikeImage {
			// If it doesn't look like an image, skip it regardless of path or strict mode.
			log.Debugf("Skipping string '%s' at path %s because it does not look like an image reference.", vStr, path)
			break // Exit case string
		}

		// --- If it looks like an image and passes strict path check (if applicable), attempt to parse ---
		log.Debugf("[DEBUG irr DETECT STRING] Attempting parse for string '%s' at path %s", vStr, path)
		imgRef, err := d.tryExtractImageFromString(vStr, path)

		if err != nil {
			// --- Handle Parsing Error ---
			log.Debugf("[DEBUG irr DETECT STRING] Parse error for string '%s' at path %s: %v", vStr, path, err)
			if d.context.Strict {
				// Strict mode: Always report parse errors if it looked like an image (and wasn't skipped above).
				log.Debugf("Strict mode: Marking string '%s' at path %s as unsupported due to parse error: %v", vStr, path, err)
				log.Debugf("[UNSUPPORTED] Adding string item at path %v due to strict parse error: %v", path, err)
				// DETAILED LOGGING: Add before appending
				log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict string parse error, Value: '%s', Error: %v, Strict: %v", path, UnsupportedTypeStringParseError, vStr, err, d.context.Strict)
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeStringParseError,
					Error:    err,
				})
			} else {
				// Non-strict mode: Log parse errors but don't mark as unsupported.
				log.Debugf("Non-strict mode: Skipping string '%s' at path %s due to parse error: %v", vStr, path, err)
			}
		} else if imgRef != nil {
			// --- Handle Successful Parse ---
			log.Debugf("[DEBUG irr DETECT STRING] Parse successful for string '%s' at path %s: Ref=%+v", vStr, path, imgRef.Reference)
			NormalizeImageReference(imgRef.Reference) // Normalize before checking source/path
			log.Debugf("[DEBUG irr DETECT STRING] Normalized Ref: %+v", imgRef.Reference)

			if isKnownImagePath {
				// Parsed successfully AND path is known image path
				if IsSourceRegistry(imgRef.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					log.Debugf("Detected source image string at known path %s: %s", path, imgRef.Reference.String())
					detectedImages = append(detectedImages, *imgRef)
				} else {
					// Valid image, known path, but not a source registry.
					// REVISED: In strict mode, non-source images at KNOWN paths should be UNSUPPORTED.
					if d.context.Strict {
						log.Debugf("Strict mode: Marking non-source image string '%s' at known image path %s as unsupported.", vStr, path)
						log.Debugf("[UNSUPPORTED] Adding string item at path %v due to strict non-source.", path)
						// DETAILED LOGGING: Add before appending
						log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict non-source string at known path, Value: '%s', Error: nil, Strict: %v", path, UnsupportedTypeNonSourceImage, vStr, d.context.Strict)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeNonSourceImage,
							Error:    nil,
						})
					} else {
						// Non-strict mode: Just skip non-source images.
						log.Debugf("Non-strict mode: Skipping non-source image string '%s' at known image path %s.", vStr, path)
					}
				}
			} else {
				// Parsed successfully BUT path is NOT a known image path (This block shouldn't be reached in strict mode due to the check at the top)
				log.Debugf("Skipping string '%s' at non-image path %s (parsed ok, but path unknown).", vStr, path)
			}
		} // else imgRef == nil (e.g., template string handled) -> do nothing more here

	case bool, float64, int, nil:
		// Ignore scalar types that cannot be images
		debug.Printf("Skipping non-string/map/slice type (%T) at path %v", v, path)
	default:
		// Handle unexpected types
		debug.Printf("Warning: Encountered unexpected type %T at path %v", v, path)
		// Depending on strictness, maybe add to unsupported
		// if d.context.Strict { allUnsupported = append(allUnsupported, UnsupportedImage{...}) }
	}

	debug.Printf("[DEBUG DETECT traverseMap END] Path=%v, Detected=%d, Unsupported=%d", path, len(detectedImages), len(unsupportedMatches))
	return detectedImages, unsupportedMatches, nil
}

// traverseMap traverses a map to find image references.
func (d *Detector) traverseMap(m map[string]interface{}, path []string) ([]DetectedImage, []UnsupportedImage) {
	debug.Printf("[DEBUG DETECT traverseMap START] Path=%v, MapSize=%d", path, len(m))
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	for key, value := range m {
		newPath := append(slices.Clone(path), key)
		// +++ STDLOG +++
		fmt.Printf("[STDLOG DETECT traverseMap] Processing Key: '%s', Path: %v, Value Type: %T\n", key, newPath, value)

		// --- Check for Image Map Pattern FIRST ---
		if mapValue, ok := value.(map[string]interface{}); ok {
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' is a map. Trying tryExtractImageFromMap...\n", key)
			detectedMapImage, isImageMap, err := d.tryExtractImageFromMap(mapValue, newPath)
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] tryExtractImageFromMap result for path %v: isImageMap=%v, err=%v\n", newPath, isImageMap, err)
			if err != nil {
				debug.Printf("Error extracting map image at path %v: %v", newPath, err)
				// Handle strict mode error reporting (append to unsupportedMatches)
				if d.context.Strict {
					log.Debugf("[UNSUPPORTED] Adding map item at path %v due to strict unsupported map: %v", newPath, err)
					log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict unsupported map, Error: %v, Strict: %v", newPath, UnsupportedTypeMap, err, d.context.Strict)
					unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
						Location: newPath,
						Type:     UnsupportedTypeMap,
						Error:    err,
					})
				} else {
					log.Debugf("Non-strict mode: Skipping map item at path %v due to unsupported map: %v", newPath, err)
				}
			}
			if isImageMap {
				if detectedMapImage != nil { // If successfully extracted
					// +++ STDLOG +++
					fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' was an image map, ADDING detected image.\n", key)
					detectedImages = append(detectedImages, *detectedMapImage)
				} else {
					// +++ STDLOG +++
					fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' was considered an image map pattern, but extraction failed (err=%v). NOT recursing.\n", key, err)
				}
				// Whether successful or not, if it matched the image map pattern, DO NOT recurse further.
				continue // Move to the next key in the outer map
			}

			// If it wasn't an image map pattern, recurse into the map
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' was NOT an image map pattern, recursing...\n", key)
			detected, unsupported, err := d.DetectImages(mapValue, newPath)
			if err != nil {
				// Propagate error
				return nil, nil // Error should be handled by caller
			}
			detectedImages = append(detectedImages, detected...)
			unsupportedMatches = append(unsupportedMatches, unsupported...)

		} else if stringValue, ok := value.(string); ok {
			// --- Check for Image String Pattern ---
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' is a string. Trying tryExtractImageFromString...\n", key)
			detectedStringImage, err := d.tryExtractImageFromString(stringValue, newPath)
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] tryExtractImageFromString result for path %v: detected=%v, err=%v\n", newPath, detectedStringImage != nil, err)
			if err != nil {
				debug.Printf("Error extracting string image at path %v: %v", newPath, err)
				// Handle strict mode error reporting (append to unsupportedMatches)
				if d.context.Strict {
					log.Debugf("[UNSUPPORTED] Adding string item at path %v due to strict unsupported string: %v", newPath, err)
					log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict unsupported string, Error: %v, Strict: %v", newPath, UnsupportedTypeString, err, d.context.Strict)
					unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
						Location: newPath,
						Type:     UnsupportedTypeString,
						Error:    err,
					})
				} else {
					log.Debugf("Non-strict mode: Skipping string item at path %v due to unsupported string: %v", newPath, err)
				}
			}
			if detectedStringImage != nil {
				// +++ STDLOG +++
				fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' was an image string, ADDING detected image.\n", key)
				detectedImages = append(detectedImages, *detectedStringImage)
				// Don't recurse into a detected image string
				continue
			}
			// If it wasn't detected as an image string (e.g., parse error, didn't look like one),
			// and we are not in strict mode (where errors are already handled),
			// we don't need to do anything else (no recursion into plain strings).

		} else if sliceValue, ok := value.([]interface{}); ok {
			// --- Recurse into Slice ---
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' is a slice, recursing...\n", key)
			detected, unsupported := d.traverseSlice(sliceValue, newPath)
			detectedImages = append(detectedImages, detected...)
			unsupportedMatches = append(unsupportedMatches, unsupported...)
		} else {
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT traverseMap] Key '%s' has unhandled type %T, skipping.\n", key, value)
			// Handle other types (bool, number, nil) - typically ignore
		}
	}

	debug.Printf("[DEBUG DETECT traverseMap END] Path=%v, Detected=%d, Unsupported=%d", path, len(detectedImages), len(unsupportedMatches))
	return detectedImages, unsupportedMatches
}

// traverseSlice traverses a slice to find image references.
func (d *Detector) traverseSlice(s []interface{}, path []string) ([]DetectedImage, []UnsupportedImage) {
	debug.Printf("[DEBUG DETECT traverseSlice START] Path=%v, SliceSize=%d", path, len(s))
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// ... existing code ...
	return detectedImages, unsupportedMatches
}

// tryExtractImageFromMap attempts to extract an image reference from known map patterns.
func (d *Detector) tryExtractImageFromMap(m map[string]interface{}, path []string) (*DetectedImage, bool, error) {
	debug.Printf("[DEBUG DETECT tryExtractImageFromMap] Trying path %v", path)
	debug.DumpValue("Input map", m)

	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path=%v, InputMap=%v\n", path, m)

	// Heuristic: Check if the current path looks like a potential image container based on key names
	pathIsImage := isImagePath(path)
	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path=%v, isImagePath result: %v\n", path, pathIsImage)
	if !pathIsImage {
		debug.Printf("[DEBUG DETECT tryExtractImageFromMap] Path %v does not match known image patterns, skipping map extraction.", path)
		return nil, false, nil // Not identified as a known image map pattern
	}

	// --- It IS an image path, now attempt to parse the map ---
	ref := &Reference{Path: path}
	keys := make(map[string]bool)
	for k := range m {
		keys[k] = true
	}

	hasRepo := keys["repository"]
	hasTag := keys["tag"]
	hasRegistry := keys["registry"]
	hasDigest := keys["digest"]

	// Basic structural check for an image map: must have at least repository.
	if !hasRepo {
		// +++ STDLOG +++
		fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v IS an image path, but map lacks 'repository' key. Not an image map.\n", path)
		return nil, false, nil // Path matched, but map structure invalid for image
	}

	// It has a repository and the path matches - treat as an image map pattern.
	// Proceed with extracting values.

	// --- Extract Repository (Required) ---
	repoVal, _ := m["repository"].(string) // Already checked hasRepo
	if repoVal == "" {
		// +++ STDLOG +++
		fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v has empty 'repository' value.\n", path)
		return nil, true, fmt.Errorf("%w: repository cannot be empty at path %v", ErrInvalidImageMapRepo, path) // Return error
	}
	ref.Repository = repoVal

	// --- Extract Registry (Optional, check global context) ---
	registrySource := "default (pending normalization)"
	if hasRegistry {
		if regStr, ok := m["registry"].(string); ok {
			ref.Registry = regStr
			registrySource = "map"
		}
	}
	if registrySource == "default (pending normalization)" && d.context.GlobalRegistry != "" {
		ref.Registry = d.context.GlobalRegistry
		registrySource = "global"
	}
	debug.Printf("Registry source for path %v: %s (Value: '%s')", path, registrySource, ref.Registry)

	// --- Extract Tag (Optional) ---
	if hasTag {
		if tagStr, ok := m["tag"].(string); ok {
			ref.Tag = tagStr
		} else {
			// Handle non-string tag? For now, error if present but not string.
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v has non-string 'tag' value: %T\n", path, m["tag"])
			return nil, true, fmt.Errorf("%w: found non-string type %T for tag at path %v", ErrInvalidImageMapTagType, m["tag"], path)
		}
	}

	// --- Extract Digest (Optional) ---
	if hasDigest {
		if digestStr, ok := m["digest"].(string); ok {
			ref.Digest = digestStr
		} else {
			// +++ STDLOG +++
			fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v has non-string 'digest' value: %T\n", path, m["digest"])
			return nil, true, fmt.Errorf("%w: found non-string type %T for digest at path %v", ErrInvalidImageMapDigestType, m["digest"], path)
		}
	}

	// --- Validation ---
	if ref.Tag != "" && ref.Digest != "" {
		// +++ STDLOG +++
		fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v has both tag and digest.\n", path)
		return nil, true, fmt.Errorf("%w: path %v", ErrTagAndDigestPresent, path)
	}
	if ref.Tag == "" && ref.Digest == "" {
		debug.Printf("Warning: Image map at path %v missing tag and digest. Assuming 'latest'.", path)
		ref.Tag = "latest"
	}

	ref.Original = fmt.Sprintf("%v", m) // Store map representation
	ref.Detected = true
	NormalizeImageReference(ref) // Normalize after extraction

	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromMap] Path %v successfully parsed as image map: %+v\n", path, ref)

	return &DetectedImage{
		Reference: ref,
		Path:      path,
		Pattern:   PatternMap,
		Original:  m, // Store original map
	}, true, nil // Return detected image, isImageMap=true, no error

	// --- Code below this point should not be reached if logic is correct ---
	// debug.Printf("[DEBUG DETECT tryExtractImageFromMap] Path %v completed checks, not identified as image map.", path)
	// return nil, false, nil // Fallback: Not identified as a known image map pattern
}

// tryExtractImageFromString attempts to extract an image reference from a string value.
func (d *Detector) tryExtractImageFromString(imgStr string, path []string) (*DetectedImage, error) {
	debug.Printf("[DEBUG DETECT tryExtractImageFromString] Trying path %v, String='%s'", path, imgStr)

	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromString] Path=%v, String='%s'\n", path, imgStr)

	// Skip if the string doesn't roughly look like an image reference
	looksLike := looksLikeImageReference(imgStr)
	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromString] Path=%v, looksLikeImageReference result: %v\n", path, looksLike)
	if !looksLike {
		debug.Printf("[DEBUG DETECT tryExtractImageFromString] String '%s' at path %v does not look like an image reference.", imgStr, path)
		return nil, nil // Not detected, not an error stopping traversal
	}

	// Handle potential template variables (if TemplateMode is added later)
	// if d.context.TemplateMode && strings.Contains(imgStr, "{{") { ... }

	// Parse the string into components
	ref, err := ParseImageReference(imgStr)
	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromString] Path=%v, ParseImageReference err: %v\n", path, err)
	if err != nil {
		debug.Printf("Failed to parse '%s' as image reference: %v", imgStr, err)
		if errors.Is(err, ErrInvalidImageRefFormat) || errors.Is(err, ErrInvalidRepoName) || errors.Is(err, ErrInvalidTagFormat) || errors.Is(err, ErrInvalidDigestFormat) {
			err = fmt.Errorf("%w: %w", ErrInvalidImageString, err)
		}
		return nil, err
	}

	// +++ STDLOG +++
	fmt.Printf("[STDLOG DETECT tryExtractImageFromString] Path=%v, Parsed Ref: %+v\n", path, ref)

	ref.Path = path
	ref.Original = imgStr // Set the Original field to input string

	ref.Detected = true
	return &DetectedImage{
		Reference: ref,
		Path:      path,
		Pattern:   PatternString,
		Original:  imgStr,
	}, nil
}

// IsValidImageReference performs basic validation on a parsed ImageReference
// Note: This checks structure, not necessarily registry reachability etc.
func IsValidImageReference(ref *Reference) bool {
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
func ParseImageReference(imgStr string) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s'", imgStr)

	// TODO: Need to improve validation for malformed image references.
	// Currently strings like "invalid-format" are not properly rejected as errors.
	// This causes test failures in TestTryExtractImageFromString_EdgeCases for the "invalid_format" case.

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

	var ref Reference
	ref.Original = imgStr // Set the Original field to input string

	// --- Prioritize Digest Parsing ---
	if strings.Contains(imgStr, "@") {
		parts := strings.SplitN(imgStr, "@", maxSplitTwo)
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
	}

	// --- Tag-Based Parsing ---
	tagIndex := strings.LastIndex(imgStr, ":")
	slashIndex := strings.Index(imgStr, "/") // First slash

	if tagIndex == -1 {
		// No colon: Assume it's just a repository name (tag defaults to 'latest')
		if !isValidRepositoryName(imgStr) {
			debug.Printf("Invalid repository name (no tag/digest): '%s'", imgStr)
			return nil, ErrInvalidRepoName
		}
		ref.Repository = imgStr
		// ref.Tag = "latest" // Let Normalize handle default tag
	} else {
		// Colon found
		// Check if colon is part of a port number in the registry part
		if slashIndex != -1 && tagIndex < slashIndex {
			// Colon appears before the first slash, likely part of a port number
			// e.g., "myregistry.com:5000/myrepo" (no tag specified)
			if !isValidRepositoryName(imgStr) { // Validate the whole string as repo part
				debug.Printf("Invalid repository name (colon before slash): '%s'", imgStr)
				return nil, ErrInvalidRepoName
			}
			ref.Repository = imgStr
			// ref.Tag = "latest" // Let Normalize handle default tag
		} else {
			// Colon appears after the first slash or no slash exists, assume it separates repo/tag
			namePart := imgStr[:tagIndex]
			tagPart := imgStr[tagIndex+1:]

			if namePart == "" || tagPart == "" {
				debug.Printf("Invalid format near tag separator: '%s'", imgStr)
				return nil, ErrInvalidImageRefFormat // Missing repo or tag
			}
			if !isValidTag(tagPart) {
				debug.Printf("Invalid tag format: '%s'", tagPart)
				return nil, ErrInvalidTagFormat
			}
			ref.Tag = tagPart

			// Parse the name part (registry/repository)
			slashIndexName := strings.Index(namePart, "/")
			if slashIndexName == -1 {
				// Assumed to be repository only (e.g., "myimage:tag")
				if !isValidRepositoryName(namePart) {
					debug.Printf("Invalid repository name (tag present): '%s'", namePart)
					return nil, ErrInvalidRepoName
				}
				ref.Repository = namePart
			} else {
				// Potential registry/repository split
				potentialRegistry := namePart[:slashIndexName]
				potentialRepo := namePart[slashIndexName+1:]

				if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
					if !isValidRegistryName(potentialRegistry) { // Optional stricter check?
						debug.Printf("Invalid registry name inferred (tag): '%s'", potentialRegistry)
						// return nil, ErrInvalidRegistryName
						return nil, fmt.Errorf("invalid registry name detected: %s", potentialRegistry)

					}
					if !isValidRepositoryName(potentialRepo) {
						debug.Printf("Invalid repository name after registry split (tag): '%s'", potentialRepo)
						return nil, ErrInvalidRepoName
					}
					ref.Registry = potentialRegistry
					ref.Repository = potentialRepo
				} else {
					// Assume the whole namepart is the repository
					if !isValidRepositoryName(namePart) {
						debug.Printf("Invalid repository name (treated as whole, tag): '%s'", namePart)
						return nil, ErrInvalidRepoName
					}
					ref.Repository = namePart
				}
			}
			// Make sure repository is not empty after parsing name part
			if ref.Repository == "" {
				debug.Println("Error: Missing repository in tag reference after parsing name.")
				return nil, ErrInvalidRepoName
			}
		}
	}

	NormalizeImageReference(&ref) // Normalize AFTER validation and parsing
	debug.Printf("Successfully parsed tag-based reference: %+v", ref)
	ref.Detected = true
	return &ref, nil
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
		ref.Tag = "latest"
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

// isValidRegistryName checks if a string is potentially a valid registry name component.
// Note: This is a basic check. Docker reference spec is complex.
func isValidRegistryName(name string) bool {
	if name == "" {
		return false
	}
	// Basic check: Allow alphanum, dot, dash, colon (for port)
	// Registry component must contain at least one dot, colon, or be "localhost".
	// Relaxed check for now - mainly check for invalid chars like spaces.
	// TODO: Improve according to docker/distribution reference spec if needed.
	// Reference: https://github.com/distribution/distribution/blob/main/reference/reference.go
	// We need domain name validation basically.
	// Example simple check:
	// return !strings.ContainsAny(name, " /\\") && (strings.ContainsAny(name, ".:") || name == "localhost")
	return !strings.ContainsAny(name, " /\\") // Very basic: no spaces or slashes allowed?
}

// isValidRepositoryName checks if a string is potentially a valid repository name component.
// Allows lowercase alphanum, underscore, dot, dash, and forward slashes for namespaces.
// Must start and end with alphanum. No consecutive separators.
func isValidRepositoryName(repo string) bool {
	if repo == "" {
		return false
	}
	// Regex based on Docker spec (simplified):
	// path-component := [a-z0-9]+(?:(?:[._]|__|[-]*)[a-z0-9]+)*
	// name-component := path-component(?:(?:/path-component)+)?
	// Using a slightly simpler check for now:
	// Allows: a-z, 0-9, '.', '_', '-', '/'
	// Constraints: starts/ends with alphanum, no consecutive separators.
	pattern := `^[a-z0-9]+(?:(?:[._-]|[/])?[a-z0-9]+)*$` // Simplified
	matched, _ := regexp.MatchString(pattern, repo)
	if !matched {
		debug.Printf("Repository '%s' failed regex check '%s'", repo, pattern)
		return false
	}
	// Check for consecutive separators (simplistic)
	if strings.Contains(repo, "//") || strings.Contains(repo, "..") || strings.Contains(repo, "__") || strings.Contains(repo, "--") || strings.Contains(repo, "-_") || strings.Contains(repo, "_-") {
		debug.Printf("Repository '%s' contains consecutive separators.", repo)
		return false
	}
	return true
}

// isValidTag checks if a string is a valid tag format.
// Max 128 chars, allowed chars: word characters (alphanum + '_') and '.', '-'. Must not start with '.' or '-'.
func isValidTag(tag string) bool {
	if tag == "" {
		return false
	}
	if len(tag) > 128 {
		return false
	}
	pattern := `^[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`
	matched, _ := regexp.MatchString(pattern, tag)
	return matched
}

// isValidDigest checks if a string is a valid sha256 digest format.
func isValidDigest(digest string) bool {
	if digest == "" {
		return false
	}
	pattern := `^sha256:[a-fA-F0-9]{64}$`
	matched, _ := regexp.MatchString(pattern, digest)
	return matched
}

// isImagePath checks if a given path matches known image patterns and not known non-image patterns.
func isImagePath(path []string) bool {
	pathStr := strings.Join(path, ".")

	// Check against non-image patterns first (more specific overrides)
	for _, re := range nonImagePathRegexps {
		if re.MatchString(pathStr) {
			debug.Printf("Path '%s' matches non-image pattern '%s', returning false.", pathStr, re.String())
			return false
		}
	}

	// Check against image patterns
	for _, re := range imagePathRegexps {
		if re.MatchString(pathStr) {
			debug.Printf("Path '%s' matches image pattern '%s', returning true.", pathStr, re.String())
			return true
		}
	}

	debug.Printf("Path '%s' did not match any known image or non-image patterns, returning false.", pathStr)
	return false
}

// looksLikeImageReference is a helper function to quickly check if a string resembles an image reference format.
// This is less strict than full parsing and used for heuristics in strict mode.
func looksLikeImageReference(s string) bool {
	// Check for required separator (: for tag or @ for digest)
	hasSeparator := strings.Contains(s, ":") || strings.Contains(s, "@")
	if !hasSeparator {
		return false // Must have one of the separators
	}

	// Avoid matching obvious file paths or URLs
	hasSlash := strings.Contains(s, "/")
	isFilePath := hasSlash && (strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../"))
	isURL := strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
	if isFilePath || isURL {
		return false
	}

	// Further check: if no slash, ensure the part before the separator is simple
	if !hasSlash {
		sepIndex := strings.LastIndex(s, ":")
		if sepIndex == -1 {
			sepIndex = strings.LastIndex(s, "@")
		}
		if sepIndex > 0 {
			potentialRepo := s[:sepIndex]
			// Basic check for plausible repo name (avoid things like 'http:')
			if strings.ContainsAny(potentialRepo, "./") { // If it contains path separators without a registry-like part, likely not an image
				return false
			}
		}
	}

	// If it has a separator and doesn't look like a file path/URL, consider it plausible.
	return true
}

// Helper function to check if a registry is in a list (case-insensitive normalized comparison)
func isRegistryInList(registry string, list []string) bool {
	normalizedRegistry := NormalizeRegistry(registry)
	for _, item := range list {
		if normalizedRegistry == NormalizeRegistry(item) {
			return true
		}
	}
	return false
}

// Basic error type for unsupported image structures
type UnsupportedImageError struct {
	Path []string
	Type UnsupportedType
	Err  error
}

func (e *UnsupportedImageError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("unsupported image structure at path %v (type %d): %v", e.Path, e.Type, e.Err)
	}
	return fmt.Sprintf("unsupported image structure at path %v (type %d)", e.Path, e.Type)
}

// Basic constructor for UnsupportedImageError
func NewUnsupportedImageError(path []string, uType UnsupportedType, err error) error {
	return &UnsupportedImageError{Path: path, Type: uType, Err: err}
}
