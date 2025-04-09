package image

import (
	"fmt"
	"strings"

	"errors"

	"github.com/lalbers/irr/pkg/debug"
	stdLog "github.com/lalbers/irr/pkg/log"
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

	// Add detailed logging for map traversal entry
	debug.Printf("[DETECTOR ENTRY] Path: %v, Type: %T", path, values)

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	switch v := values.(type) {
	case map[string]interface{}:
		debug.Println("Processing map")

		// First, try to detect an image map at the current level
		detectedImage, isImage, err := d.tryExtractImageFromMap(v, path)
		if isImage {
			// --- If isImageMap was true ---
			// Structure looked like an image map attempt.
			// Check for errors, then validity.
			switch {
			// Check for errors FIRST
			case err != nil:
				// Handle errors returned by tryExtractImageFromMap (e.g., template variables in strict mode)
				stdLog.Debugf("[UNSUPPORTED ADD - Map Error] Path: %v, Error: %v\n", path, err)
				// Determine the type code based on the error
				var typeCode = UnsupportedTypeMapError // Default
				if errors.Is(err, ErrTemplateVariableDetected) {
					typeCode = UnsupportedTypeTemplateMap
				} else if errors.Is(err, ErrTagAndDigestPresent) {
					typeCode = UnsupportedTypeMapTagAndDigest
				} // Add other specific error checks if needed
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     typeCode,
					Error:    err,
				})
			// Handle nil image second (e.g., non-strict skips, empty repo map)
			case detectedImage == nil:
				stdLog.Debugf("Skipping nil detectedImage at path %v despite isImageMap being true. This might indicate an empty or incomplete map structure skipped in non-strict mode.", path)
				// Do nothing, just don't recurse (handled by isImageMap=true check later)
			// Check validity if no error and image is non-nil
			case detectedImage.Reference != nil && IsValidImageReference(detectedImage.Reference):
				// Valid reference found (detectedImage is implicitly non-nil here)
				if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					debug.Printf("Detected map-based image at path %v: %v\n", path, detectedImage.Reference)
					detectedImages = append(detectedImages, *detectedImage)
				} else {
					debug.Printf("Map-based image is not a source registry at path %v: %v\n", path, detectedImage.Reference)
					if d.context.Strict {
						stdLog.Debugf("[UNSUPPORTED] Adding map item at path %v due to strict non-source.\n", path)
						stdLog.Debugf("[UNSUPPORTED ADD] Path: %v, Type: %v, Reason: Strict non-source map, Error: nil, Strict: %v\n",
							path, UnsupportedTypeNonSourceImage, d.context.Strict)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeNonSourceImage,
							Error:    fmt.Errorf("strict mode: map image at path %v is not from a configured source registry", path), // Added more context to error
						})
					} else {
						stdLog.Debugf("Non-strict mode: Skipping non-source map image at path %v\n", path)
					}
				}
			default: // Covers: Invalid Reference AFTER normalization OR detectedImage.Reference was nil (should not happen if cases above are correct)
				stdLog.Debugf("Skipping invalid/nil map-based image reference at path %v: Ref=%+v", path, detectedImage.Reference)
				// Treat invalid map structure as unsupported in strict mode.
				if d.context.Strict {
					stdLog.Debugf("[UNSUPPORTED ADD - Strict Invalid Map] Path: %v, Type: %v\n", path, UnsupportedTypeMapParseError)
					unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
						Location: path,
						Type:     UnsupportedTypeMapParseError,
						Error:    fmt.Errorf("strict mode: map structure at path %v was invalid after normalization", path),
					})
				}
			}
			// Whether detected, skipped non-source, or invalid, if it was identified AS an image map structure attempt (isImageMap == true),
			// we should NOT recurse further into its values.
			return detectedImages, unsupportedMatches, nil
		}

		// --- If isImageMap was false ---
		stdLog.Debugf("Structure at path %s did not match image map pattern, recursing into values.\n", pathToString(path))
		for key, val := range v {
			newPath := append(append([]string{}, path...), key)
			detected, unsupported, err := d.DetectImages(val, newPath)
			if err != nil {
				stdLog.Errorf("Error processing path %v: %v\n", newPath, err)
				return nil, nil, fmt.Errorf("error processing path %v: %w\n", newPath, err)
			}
			detectedImages = append(detectedImages, detected...)
			if len(unsupported) > 0 {
				stdLog.Debugf("[UNSUPPORTED AGG MAP] Path: %v, Appending %d items from key '%s'\n", path, len(unsupported), key)
				for i, item := range unsupported {
					stdLog.Debugf("[UNSUPPORTED AGG MAP ITEM %d] Path: %v, Type: %v, Error: %v\n", i, item.Location, item.Type, item.Error)
				}
			}
			unsupportedMatches = append(unsupportedMatches, unsupported...)
		}

	case []interface{}:
		debug.Println("Processing slice/array")
		// Always process arrays, path check should happen for items within if needed
		for i, item := range v {
			itemPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i)) // Ensure path is copied, removed erroneous newline
			stdLog.Debugf("Recursively processing slice item %d at path %s\n", i, itemPath)
			detectedInItem, unsupportedInItem, err := d.DetectImages(item, itemPath)
			if err != nil {
				stdLog.Errorf("Error processing slice item at path %s: %v\n", itemPath, err)
				return nil, nil, fmt.Errorf("error processing slice item %d at path %s: %w\n", i, path, err)
			}
			detectedImages = append(detectedImages, detectedInItem...)
			if len(unsupportedInItem) > 0 {
				stdLog.Debugf("[UNSUPPORTED AGG SLICE] Path: %v, Appending %d items from index %d\n", path, len(unsupportedInItem), i)
				for j, item := range unsupportedInItem {
					stdLog.Debugf("[UNSUPPORTED AGG SLICE ITEM %d] Path: %v, Type: %v, Error: %v\n", j, item.Location, item.Type, item.Error)
				}
			}
			unsupportedMatches = append(unsupportedMatches, unsupportedInItem...)
		}

	case string:
		vStr := v
		stdLog.Debugf("[DEBUG irr DETECT STRING] Processing string value at path %s: %q\n", path, vStr)
		stdLog.Debugf("[DEBUG irr DETECT STRING] Path: %v, Value: '%s', Strict Context: %v\n", path, vStr, d.context.Strict)

		isKnownImagePath := isImagePath(path)

		if d.context.Strict {
			// --- Strict Mode Logic ---

			// 1. Check for templates FIRST, regardless of path.
			if containsTemplate(vStr) {
				stdLog.Debugf("Strict mode: Marking string '%s' at path %s as unsupported due to template variable detected.\n", vStr, path)
				stdLog.Debugf("[UNSUPPORTED ADD - Strict Template String] Path: %v, Type: %v, Value: '%s'\n", path, UnsupportedTypeTemplateString, vStr)
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeTemplateString,
					Error:    fmt.Errorf("strict mode: template variable detected in string at path %v", path),
				})
				break // Template found, stop processing this string.
			}

			// 2. If NO template, check if path is known.
			if !isKnownImagePath {
				stdLog.Debugf("Strict mode: Skipping non-template string at unknown image path %s: %q\n", path, vStr)
				break // Skip non-template strings at unknown paths
			}

			// 3. If NO template AND path IS known, proceed with parsing and validation.
			stdLog.Debugf("Strict mode & Known path: Attempting parse for non-template string '%s' at path %s\n", vStr, path)
			imgRef, err := d.tryExtractImageFromString(vStr, path)

			if err != nil {
				// Marks parse errors as unsupported - OK
				stdLog.Debugf("Strict mode: Marking string '%s' at known image path %s as unsupported due to parse error: %v\n", vStr, path, err)
				stdLog.Debugf("[UNSUPPORTED ADD - Strict Parse Fail] Path: %v, Type: %v, Value: '%s', Error: %v\n", path, UnsupportedTypeStringParseError, vStr, err)
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeStringParseError,
					Error:    fmt.Errorf("strict mode: string at known image path %v failed to parse: %w", path, err),
				})
			} else if imgRef != nil {
				NormalizeImageReference(imgRef.Reference)

				// Check source/exclude lists AFTER normalization
				isSource := IsSourceRegistry(imgRef.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries)

				if isSource { // Original logic for other paths
					stdLog.Debugf("Strict mode: Detected source image string at known path %s: %s\n", path, imgRef.Reference.String())
					detectedImages = append(detectedImages, *imgRef)
				} else {
					// It's not a source registry, but WAS it excluded?
					isExcluded := false
					regNorm := NormalizeRegistry(imgRef.Reference.Registry)
					for _, exclude := range d.context.ExcludeRegistries {
						if regNorm == NormalizeRegistry(exclude) {
							isExcluded = true
							break
						}
					}

					if isExcluded {
						stdLog.Debugf("Strict mode: Marking EXCLUDED image string '%s' at known image path %s as unsupported.\n", vStr, path)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeExcludedImage,
							Error:    fmt.Errorf("strict mode: image at known path %v is from an excluded registry", path),
						})
					} else {
						stdLog.Debugf("Strict mode: Marking NON-SOURCE image string '%s' at known image path %s as unsupported.\n", vStr, path)
						unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
							Location: path,
							Type:     UnsupportedTypeNonSourceImage,
							Error:    fmt.Errorf("strict mode: image at known path %v is not from a configured source registry", path),
						})
					}
				}
			} // else imgRef == nil (shouldn't happen if err is nil, but ignore if it does)

		} else {
			// --- Non-Strict Mode Logic (remains the same) ---

			// Only attempt parse if it looks like an image AND (implicitly) we are not in strict mode at a known path
			looksLikeImage := looksLikeImageReference(vStr)
			if !looksLikeImage {
				stdLog.Debugf("Non-strict: Skipping string '%s' at path %s because it does not look like an image reference.\n", vStr, path)
				break // Skip if it doesn't look like an image
			}

			// It looks like an image, attempt to parse.
			stdLog.Debugf("Non-strict: Attempting parse for string '%s' at path %s\n", vStr, path)
			imgRef, err := d.tryExtractImageFromString(vStr, path)

			if err != nil {
				// Non-strict mode: Log parse errors but don't mark as unsupported.
				stdLog.Debugf("Non-strict: Skipping string '%s' at path %s due to parse error: %v\n", vStr, path, err)
				break // Skip on parse error
			}

			if imgRef != nil {
				// Successful parse -> Check path and source
				NormalizeImageReference(imgRef.Reference) // Normalize before checking
				stdLog.Debugf("[DEBUG irr DETECT STRING - Non-Strict] Normalized Ref: %+v\n", imgRef.Reference)
				if isKnownImagePath && IsSourceRegistry(imgRef.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
					// Only add if path is known AND it's a source registry
					stdLog.Debugf("Non-strict: Detected source image string at known path %s: %s\n", path, imgRef.Reference.String())
					detectedImages = append(detectedImages, *imgRef)
				} else {
					// Skip if path unknown OR not a source registry
					if !isKnownImagePath {
						stdLog.Debugf("Non-strict: Skipping string '%s' at non-image path %s (parsed ok, but path unknown).\n", vStr, path)
					} else {
						stdLog.Debugf("Non-strict: Skipping non-source image string '%s' at known image path %s.\n", vStr, path)
					}
				}
			} // else imgRef == nil -> do nothing
		}

	case bool, float64, int, nil:
		debug.Printf("Skipping non-string/map/slice type (%T) at path %v", v, path)
	default:
		debug.Printf("Warning: Encountered unexpected type %T at path %v", v, path)
		// Depending on strictness, maybe add to unsupported
		// if d.context.Strict { allUnsupported = append(allUnsupported, UnsupportedImage{...}) }
	}

	debug.Printf("[DEBUG DETECT traverse END] Path=%v, Detected=%d, Unsupported=%d", path, len(detectedImages), len(unsupportedMatches))
	return detectedImages, unsupportedMatches, nil
}

// tryExtractImageFromMap checks if a map conforms to a known image structure.
// Returns the parsed *DetectedImage*, a boolean indicating if it structurally matched, and an error.
func (d *Detector) tryExtractImageFromMap(m map[string]interface{}, path []string) (*DetectedImage, bool, error) {
	debug.FunctionEnter("tryExtractImageFromMap")
	defer debug.FunctionExit("tryExtractImageFromMap")
	debug.DumpValue("Input map", m)
	debug.Printf("Path: %v", path)

	// --- Check for required keys and templates first ---

	// Check repository presence and type
	repoVal, repoPresent := m["repository"]
	if !repoPresent {
		// Repository key missing entirely - not an image map structure.
		debug.Printf("[tryExtractImageFromMap] Map at path %v lacks 'repository' key. Not an image map structure.\n", pathToString(path))
		return nil, false, nil // isImageMap = false
	}

	repoValStr, repoIsString := repoVal.(string)
	if !repoIsString {
		// Repository key present, but wrong type. THIS IS an image map attempt, but invalid.
		debug.Printf("[tryExtractImageFromMap] Map at path %v has non-string 'repository' key (type %T). Invalid image map structure.\n", pathToString(path), repoVal)
		if d.context.Strict {
			// Return specific error for invalid type in strict mode
			err := fmt.Errorf("image map has invalid repository type (must be string): found type %T", repoVal)
			return nil, true, err // isImageMap = true, return error
		}
		// In non-strict, treat as a map structure but skip detection.
		return nil, true, nil // isImageMap = true, return nil error
	}

	// Now check other keys (presence and type assertion in one step is ok here)
	tagValStr, tagOk := m["tag"].(string)
	regValStr, regOk := m["registry"].(string)
	digestValStr, digestOk := m["digest"].(string)

	// Check for templates *regardless* of path, but only if keys exist
	templateFound := false
	if repoIsString && containsTemplate(repoValStr) { // Already know it's a string
		debug.Printf("Template variable found in map key 'repository': '%s'", repoValStr)
		templateFound = true
	}
	if tagOk && containsTemplate(tagValStr) {
		debug.Printf("Template variable found in map key 'tag': '%s'", tagValStr)
		templateFound = true
	}
	if regOk && containsTemplate(regValStr) {
		debug.Printf("Template variable found in map key 'registry': '%s'", regValStr)
		templateFound = true
	}
	if digestOk && containsTemplate(digestValStr) {
		debug.Printf("Template variable found in map key 'digest': '%s'", digestValStr)
		templateFound = true
	}

	if templateFound {
		debug.Printf("[MAP DEBUG] Returning ErrTemplateVariableDetected because templateFound=true")
		// Indicate structure match (isImageMap=true) but return nil *DetectedImage and the error
		return nil, true, ErrTemplateVariableDetected
	}

	// --- If no templates found, proceed with structural validation ---

	// Now we know repoValStr is a valid string. Check if it's empty.
	if repoValStr == "" {
		debug.Printf("Path [%s] has empty 'repository' value.\n", pathToString(path))
		if d.context.Strict {
			reason := fmt.Errorf("repository cannot be empty at path %v", path)
			// Indicate isImageMap = true, return nil *DetectedImage and the error.
			return nil, true, fmt.Errorf("image map validation failed: %w", reason)
		}
		// In non-strict, treat as potentially valid structure but skip detection.
		// Indicate isImageMap=true, return nil *DetectedImage and nil error.
		return nil, true, nil
	}

	// --- If we get here: No templates, has required repo key with non-empty value. ---
	// It IS considered an image map structure attempt.

	// --- Value Validation & Reference Creation ---
	parsedRef := &Reference{
		Repository: repoValStr,
		Path:       copyPath(path),
		Original:   fmt.Sprintf("%v", m),
	}

	// Handle registry: value from map takes precedence over global
	if regOk && regValStr != "" {
		if !isValidRegistryName(regValStr) {
			debug.Printf("Invalid registry name in map: '%s'", regValStr)
			if d.context.Strict {
				return nil, true, fmt.Errorf("invalid registry name '%s' in map at path %v", regValStr, path)
			}
			return nil, true, nil // Skip in non-strict
		}
		parsedRef.Registry = regValStr
		debug.Printf("Registry set from map key at path %v: %s", path, parsedRef.Registry)
	} else if d.context.GlobalRegistry != "" {
		// If registry not set in map, use global if available
		parsedRef.Registry = d.context.GlobalRegistry
		debug.Printf("Registry set from global context at path %v: %s", path, parsedRef.Registry)
	} // else: registry not in map and no global - will be defaulted during normalization

	// Validate tag if present
	if tagOk {
		if tagValStr != "" {
			if !isValidTag(tagValStr) {
				debug.Printf("Invalid tag in map: '%s'", tagValStr)
				if d.context.Strict {
					return nil, true, fmt.Errorf("invalid tag '%s' in map at path %v", tagValStr, path)
				}
				return nil, true, nil // Skip in non-strict
			}
			parsedRef.Tag = tagValStr
		} // else: tag key exists but is empty string - allowed
	} // else: tag key doesn't exist - allowed

	// Validate digest if present
	if digestOk {
		if digestValStr != "" {
			if !isValidDigest(digestValStr) {
				debug.Printf("Invalid digest in map: '%s'", digestValStr)
				if d.context.Strict {
					return nil, true, fmt.Errorf("invalid digest '%s' in map at path %v", digestValStr, path)
				}
				return nil, true, nil // Skip in non-strict
			}
			parsedRef.Digest = digestValStr
		} // else: digest key exists but is empty string - allowed
	} // else: digest key doesn't exist - allowed

	// Final check: cannot have both tag and digest
	if parsedRef.Tag != "" && parsedRef.Digest != "" {
		debug.Printf("Map contains both tag and digest at path %v", path)
		if d.context.Strict {
			return nil, true, fmt.Errorf("%w: map at path %v contains both tag ('%s') and digest ('%s')", ErrTagAndDigestPresent, path, parsedRef.Tag, parsedRef.Digest)
		}
		return nil, true, nil // Skip in non-strict
	}

	// If we got here, the structure and values are valid (or skipped non-strictly)
	NormalizeImageReference(parsedRef) // Normalize before returning

	// Check validity *after* normalization
	if !IsValidImageReference(parsedRef) {
		debug.Printf("Map at path %v is invalid after normalization: %+v", path, parsedRef)
		if d.context.Strict {
			return nil, true, fmt.Errorf("map at path %v is invalid after normalization", path)
		}
		// Skip invalid normalized ref in non-strict
		return nil, true, nil
	}

	debug.Printf("Path [%s] successfully parsed as image map: %s", pathToString(path), parsedRef.String())
	parsedRef.Detected = true

	// Create DetectedImage struct
	detected := &DetectedImage{
		Reference:      parsedRef,
		Path:           copyPath(path), // Ensure path is copied
		Pattern:        "map",
		Original:       m,     // Store original map
		OriginalFormat: "map", // Set original format
	}

	// +++ Add Debugging +++
	debug.Printf("[DETECTOR DEBUG tryExtractImageFromMap] Returning DetectedImage for path %v with OriginalFormat: '%s'", detected.Path, detected.OriginalFormat)
	debug.DumpValue("[DETECTOR DEBUG tryExtractImageFromMap] DetectedImage Value", detected)

	return detected, true, nil
}

// tryExtractImageFromString attempts to parse a string value as a Docker image reference.
// Returns the parsed DetectedImage or nil if it's not a valid image format.
// Returns ErrTemplateVariableDetected if a template is found.
func (d *Detector) tryExtractImageFromString(s string, path []string) (*DetectedImage, error) {
	debug.FunctionEnter("tryExtractImageFromString")
	defer debug.FunctionExit("tryExtractImageFromString")
	debug.Printf("Path='%v', String='%s'", path, s)

	// Check for template variables first
	if containsTemplate(s) {
		debug.Printf("Template variable detected in string: '%s'", s)
		// In strict mode, this is an error we want to report upstream.
		// In non-strict mode, we just return nil, nil to skip it.
		if d.context.Strict {
			return nil, ErrTemplateVariableDetected
		}
		return nil, ErrSkippedTemplateDetection // Return sentinel error instead of nil, nil
	}

	ref, err := ParseImageReference(s)
	if err != nil {
		debug.Printf("ParseImageReference err: %v", err)
		// Wrap the error to provide context about the failure type if needed upstream
		// We retain the original error type using %w
		return nil, fmt.Errorf("invalid image string format: %w", err)
	}

	// Parsed successfully
	ref.Path = copyPath(path) // Store path where string was detected
	ref.Original = s          // Store original string in Reference
	ref.Detected = true
	debug.Printf("Parsed Ref: %s", ref.String())

	// Create DetectedImage struct
	detected := &DetectedImage{
		Reference:      ref,
		Path:           copyPath(path), // Ensure path is copied
		Pattern:        "string",
		Original:       s,        // Store original string
		OriginalFormat: "string", // Set original format
	}

	// +++ Add Debugging +++
	debug.Printf("[DETECTOR DEBUG tryExtractImageFromString] Returning DetectedImage for path %v with OriginalFormat: '%s'", detected.Path, detected.OriginalFormat)
	debug.DumpValue("[DETECTOR DEBUG tryExtractImageFromString] DetectedImage Value", detected)

	return detected, nil
}

// containsTemplate checks if a string contains Go template syntax.
func containsTemplate(s string) bool {
	hasOpen := strings.Contains(s, "{{")
	hasClose := strings.Contains(s, "}}")
	result := hasOpen && hasClose
	debug.Printf("[containsTemplate DEBUG] String: '%s', HasOpen: %v, HasClose: %v, Result: %v", s, hasOpen, hasClose, result)
	return result
}

// addUnsupportedMatch is a helper to add an item to the unsupported list,
// providing context about the path and value.
func (d *Detector) addUnsupportedMatch(
	matches []UnsupportedImage,
	path []string,
	value interface{},
	reason error, // Keep 'reason' name for clarity, maps to 'Error' field
	uType UnsupportedType, // Changed parameter name and type
) []UnsupportedImage {
	// Create a string representation of the value for the report
	var valueStr string
	if str, ok := value.(string); ok {
		valueStr = str
	} else {
		// For non-strings (like maps), use a compact representation
		valueStr = fmt.Sprintf("%v", value) // Consider more sophisticated marshaling if needed
	}

	match := UnsupportedImage{
		Location: copyPath(path), // Use Location field, copy path
		Type:     uType,          // Use Type field and corrected parameter name
		Error:    reason,         // Use Error field
	}
	// Keep the detailed log including the valueStr for debugging
	// Use uType (which is an int underlying) for the %s format specifier in the log message context
	debug.Printf("[UNSUPPORTED ADD - %s] Path: %v, Type: %v, Value: '%s', Error: %v", uType, path, fmt.Sprintf("%T", value), valueStr, reason)
	return append(matches, match)
}

// --- Utility Functions ---

// copyPath creates a new slice with the same elements as the input path.
// This is crucial to avoid modifications to the path slice shared across recursive calls.
func copyPath(p []string) []string {
	if p == nil {
		return nil // Handle nil input gracefully
	}
	newPath := make([]string, len(p))
	copy(newPath, p)
	return newPath
}

// pathToString converts a path slice to a dot-separated string for logging.
func pathToString(path []string) string {
	return strings.Join(path, ".")
}

// isSourceRegistry checks if the given registry (or lack thereof, implying docker.io)
// matches any of the configured source registries.
func isSourceRegistry(registry string, sourceRegistries []string) bool {
	normalizedRegistry := NormalizeRegistry(registry) // Normalize before comparison
	for _, source := range sourceRegistries {
		if normalizedRegistry == NormalizeRegistry(source) {
			return true
		}
	}
	return false
}

// isRegistryExcluded checks if the given registry matches any exclusion patterns.
func isRegistryExcluded(registry string, excludeRegistries []string) bool {
	normalizedRegistry := NormalizeRegistry(registry)
	for _, exclude := range excludeRegistries {
		if normalizedRegistry == NormalizeRegistry(exclude) {
			return true
		}
	}
	return false
}

// hasCommonImageKeys checks if a map contains keys commonly associated with image definitions.
func hasCommonImageKeys(m map[string]interface{}) bool {
	_, hasRepo := m["repository"]
	_, hasImage := m["image"] // Less specific, but common
	// Add more keys if needed (e.g., "imageName")
	return hasRepo || hasImage
}
