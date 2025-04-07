package image

import (
	"errors"
	"fmt"

	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/log"
)

// Detector provides methods for finding image references within complex data structures.
type Detector struct {
	context *DetectionContext
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
				log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Map extraction error, Error: %v, Strict: %v",
					path, UnsupportedTypeMap, err, d.context.Strict)
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
						log.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict non-source map, Error: nil, Strict: %v",
							path, UnsupportedTypeNonSourceImage, d.context.Strict)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeNonSourceImage,
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
