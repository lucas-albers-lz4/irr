package image

import (
	"fmt"
	"strings"

	"errors"

	"github.com/lalbers/irr/pkg/debug"
	log "github.com/lalbers/irr/pkg/log"
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
	debug.Printf("[DETECTOR ENTRY] Path: %v, Type: %T", path, values)

	switch v := values.(type) {
	case map[string]interface{}:
		return d.processMapValue(v, path)
	case []interface{}:
		return d.processSliceValue(v, path)
	case string:
		return d.processStringValue(v, path)
	default:
		return nil, nil, nil
	}
}

// processMapValue handles detection of images in map values
func (d *Detector) processMapValue(v map[string]interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	debug.Println("Processing map")
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// First, try to detect an image map at the current level
	detectedImage, isImage, err := d.tryExtractImageFromMap(v, path)
	if isImage {
		return d.handleImageMap(detectedImage, err, path)
	}

	// If not an image map, recurse into values
	log.Debugf("Structure at path %s did not match image map pattern, recursing into values.\n", pathToString(path))
	for key, val := range v {
		newPath := append(append([]string{}, path...), key)
		detected, unsupported, err := d.DetectImages(val, newPath)
		if err != nil {
			log.Errorf("Error processing path %v: %v\n", newPath, err)
			return nil, nil, fmt.Errorf("error processing path %v: %w\n", newPath, err)
		}
		detectedImages = append(detectedImages, detected...)
		if len(unsupported) > 0 {
			log.Debugf("[UNSUPPORTED AGG MAP] Path: %v, Appending %d items from key '%s'\n", path, len(unsupported), key)
			for i, item := range unsupported {
				log.Debugf("[UNSUPPORTED AGG MAP ITEM %d] Path: %v, Type: %v, Error: %v\n", i, item.Location, item.Type, item.Error)
			}
		}
		unsupportedMatches = append(unsupportedMatches, unsupported...)
	}
	return detectedImages, unsupportedMatches, nil
}

// handleImageMap processes a detected image map
func (d *Detector) handleImageMap(detectedImage *DetectedImage, err error, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	switch {
	case err != nil:
		// Handle errors returned by tryExtractImageFromMap
		log.Debugf("[UNSUPPORTED ADD - Map Error] Path: %v, Error: %v\n", path, err)
		typeCode := d.determineUnsupportedTypeCode(err)
		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     typeCode,
			Error:    err,
		})
	case detectedImage == nil:
		log.Debugf("Skipping nil detectedImage at path %v despite isImageMap being true.", path)
	case detectedImage.Reference != nil && IsValidImageReference(detectedImage.Reference):
		if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
			debug.Printf("Detected map-based image at path %v: %v\n", path, detectedImage.Reference)
			detectedImages = append(detectedImages, *detectedImage)
		} else {
			debug.Printf("Map-based image is not a source registry at path %v: %v\n", path, detectedImage.Reference)
			if d.context.Strict {
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeNonSourceImage,
					Error:    fmt.Errorf("strict mode: map image at path %v is not from a configured source registry", path),
				})
			}
		}
	default:
		if d.context.Strict {
			unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
				Location: path,
				Type:     UnsupportedTypeMapParseError,
				Error:    fmt.Errorf("strict mode: map structure at path %v was invalid after normalization", path),
			})
		}
	}
	return detectedImages, unsupportedMatches, nil
}

// processSliceValue handles detection of images in slice/array values
func (d *Detector) processSliceValue(v []interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	debug.Println("Processing slice/array")
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	for i, item := range v {
		itemPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
		log.Debugf("Recursively processing slice item %d at path %s\n", i, itemPath)
		detectedInItem, unsupportedInItem, err := d.DetectImages(item, itemPath)
		if err != nil {
			log.Errorf("Error processing slice item at path %s: %v\n", itemPath, err)
			return nil, nil, fmt.Errorf("error processing slice item %d at path %s: %w\n", i, path, err)
		}
		detectedImages = append(detectedImages, detectedInItem...)
		if len(unsupportedInItem) > 0 {
			log.Debugf("[UNSUPPORTED AGG SLICE] Path: %v, Appending %d items from index %d\n", path, len(unsupportedInItem), i)
			for j, item := range unsupportedInItem {
				log.Debugf("[UNSUPPORTED AGG SLICE ITEM %d] Path: %v, Type: %v, Error: %v\n", j, item.Location, item.Type, item.Error)
			}
		}
		unsupportedMatches = append(unsupportedMatches, unsupportedInItem...)
	}
	return detectedImages, unsupportedMatches, nil
}

// processStringValue handles detection of images in string values
func (d *Detector) processStringValue(v string, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	log.Debugf("[DEBUG irr DETECT STRING] Processing string value at path %s: %q\n", path, v)
	log.Debugf("[DEBUG irr DETECT STRING] Path: %v, Value: '%s', Strict Context: %v\n", path, v, d.context.Strict)

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	isKnownImagePath := isImagePath(path)

	if d.context.Strict {
		// Strict mode still uses isKnownImagePath
		return d.processStringValueStrict(v, path, isKnownImagePath)
	}

	// Non-strict mode processing: Attempt to parse any string, ignore isKnownImagePath
	imgRef, err := d.tryExtractImageFromString(v, path)
	if err != nil {
		// If it's a skipped template, don't return error, just no images
		if errors.Is(err, ErrSkippedTemplateDetection) {
			return nil, nil, nil
		}
		// For other errors (like actual parse errors), return the error.
		// This could happen if a string looks *like* an image but is invalid.
		return nil, nil, err
	}
	if imgRef != nil && IsSourceRegistry(imgRef.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
		detectedImages = append(detectedImages, *imgRef)
	}

	return detectedImages, unsupportedMatches, nil
}

// processStringValueStrict handles string processing in strict mode
func (d *Detector) processStringValueStrict(v string, path []string, isKnownImagePath bool) ([]DetectedImage, []UnsupportedImage, error) {
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// 1. Check for templates first
	if containsTemplate(v) {
		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     UnsupportedTypeTemplateString,
			Error:    fmt.Errorf("strict mode: template variable detected in string at path %v", path),
		})
		return detectedImages, unsupportedMatches, nil
	}

	// 2. Check if path is known
	if !isKnownImagePath {
		return detectedImages, unsupportedMatches, nil
	}

	// 3. Parse and validate
	imgRefDetected, err := d.tryExtractImageFromString(v, path)
	if err != nil {
		var unsupportedType UnsupportedType
		var errMsg string

		// Check the specific error type returned by tryExtractImageFromString
		if errors.Is(err, ErrTemplateVariableDetected) {
			unsupportedType = UnsupportedTypeTemplateString
			errMsg = fmt.Sprintf("strict mode: template variable detected in string at path %v", path)
		} else {
			// Assume other errors are parsing errors
			unsupportedType = UnsupportedTypeStringParseError
			errMsg = fmt.Sprintf("strict mode: string at known image path %v failed to parse: %v", path, err)
			// Note: We wrap the original error (err) in the UnsupportedImage.Error below
		}

		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     unsupportedType,
			Error:    fmt.Errorf(errMsg+": %w", err), // Wrap original error for context
		})
		return detectedImages, unsupportedMatches, nil
	}

	// If err is nil, proceed with source registry check
	if imgRefDetected != nil {
		NormalizeImageReference(imgRefDetected.Reference)
		if IsSourceRegistry(imgRefDetected.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
			detectedImages = append(detectedImages, *imgRefDetected)
		} else {
			unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
				Location: path,
				Type:     UnsupportedTypeNonSourceImage,
				Error:    fmt.Errorf("strict mode: string at path %v is not from a configured source registry", path),
			})
		}
	}

	return detectedImages, unsupportedMatches, nil
}

// determineUnsupportedTypeCode determines the type code for unsupported images
func (d *Detector) determineUnsupportedTypeCode(err error) UnsupportedType {
	if errors.Is(err, ErrTemplateVariableDetected) {
		return UnsupportedTypeTemplateMap
	}
	if errors.Is(err, ErrTagAndDigestPresent) {
		return UnsupportedTypeMapTagAndDigest
	}
	return UnsupportedTypeMapError
}

// validateMapStructure checks if a map has the required image structure
func (d *Detector) validateMapStructure(m map[string]interface{}, path []string) (string, string, string, string, bool, error) {
	// Extract values and validate types
	repoVal, repoOk := m["repository"]
	regVal, regOk := m["registry"]
	tagVal, tagOk := m["tag"]
	digestVal, digestOk := m["digest"]

	// Check repository key presence
	if !repoOk {
		debug.Printf("Path [%s] missing required 'repository' key.\n", pathToString(path))
		return "", "", "", "", false, nil
	}

	// Validate value types
	repoValStr, repoIsStr := repoVal.(string)
	if !repoIsStr {
		debug.Printf("Path [%s] 'repository' value is not a string.\n", pathToString(path))
		return "", "", "", "", false, nil
	}

	// Convert other values to strings if present, ensuring correct type if key exists
	regValStr := ""
	if regOk {
		if str, ok := regVal.(string); ok {
			regValStr = str
		} else {
			// Key exists but value is not a string
			debug.Printf("Path [%s] 'registry' value exists but is not a string.\n", pathToString(path))
			return "", "", "", "", false, nil
		}
	}

	tagValStr := ""
	if tagOk {
		if str, ok := tagVal.(string); ok {
			tagValStr = str
		} else {
			// Key exists but value is not a string
			debug.Printf("Path [%s] 'tag' value exists but is not a string.\n", pathToString(path))
			return "", "", "", "", false, nil
		}
	}

	digestValStr := ""
	if digestOk {
		if str, ok := digestVal.(string); ok {
			digestValStr = str
		} else {
			// Key exists but value is not a string
			debug.Printf("Path [%s] 'digest' value exists but is not a string.\n", pathToString(path))
			return "", "", "", "", false, nil
		}
	}

	return repoValStr, regValStr, tagValStr, digestValStr, true, nil
}

// checkForTemplates checks if any map values contain template expressions
func (d *Detector) checkForTemplates(repoStr, regStr, tagStr, digestStr string) error {
	if containsTemplate(repoStr) {
		debug.Printf("Template variable found in map key 'repository': '%s'", repoStr)
		return ErrTemplateVariableDetected
	}
	if regStr != "" && containsTemplate(regStr) {
		debug.Printf("Template variable found in map key 'registry': '%s'", regStr)
		return ErrTemplateVariableDetected
	}
	if digestStr != "" && containsTemplate(digestStr) {
		debug.Printf("Template variable found in map key 'digest': '%s'", digestStr)
		return ErrTemplateVariableDetected
	}
	if tagStr != "" && containsTemplate(tagStr) {
		debug.Printf("Template variable found in map key 'tag': '%s'", tagStr)
		return ErrTemplateVariableDetected
	}
	return nil
}

// createImageReference creates and validates an image reference from map values
func (d *Detector) createImageReference(repoStr, regStr, tagStr, digestStr string, path []string) (*Reference, error) {
	// Combine components into a potential reference string
	var fullRefStr string
	if regStr != "" {
		fullRefStr = regStr + "/" + repoStr
	} else {
		fullRefStr = repoStr
	}

	// Append tag or digest if provided and valid
	if tagStr != "" && digestStr != "" {
		return nil, fmt.Errorf("%w: map at path %v has both tag ('%s') and digest ('%s')",
			ErrTagAndDigestPresent, path, tagStr, digestStr)
	} else if tagStr != "" {
		if !isValidTag(tagStr) {
			return nil, fmt.Errorf("invalid tag format ('%s') in map at path %v", tagStr, path)
		}
		fullRefStr += ":" + tagStr
	} else if digestStr != "" {
		if !isValidDigest(digestStr) {
			return nil, fmt.Errorf("invalid digest format ('%s') in map at path %v", digestStr, path)
		}
		fullRefStr += "@" + digestStr
	} else {
		// No tag or digest explicitly provided in map structure
	}

	// If repoStr already contains tag/digest, use it directly after validation
	// This handles cases like `repository: myimage:latest` or `repository: myimage@sha256:...`
	// We pass strict mode here as well.
	if strings.Contains(repoStr, ":") || strings.Contains(repoStr, "@") {
		ref, err := ParseImageReference(repoStr, d.context.Strict) // Pass strict mode
		if err == nil {
			// Successfully parsed from repoStr, potentially overriding separate tag/digest fields
			ref.Path = copyPath(path)
			ref.Original = fmt.Sprintf("repository=%s,tag=%s,digest=%s (parsed from repository field)", repoStr, tagStr, digestStr)
			ref.Detected = true
			NormalizeImageReference(ref) // Normalize potential defaults (e.g., missing tag)
			if !IsValidImageReference(ref) {
				return nil, fmt.Errorf("invalid map structure at path %v resulted in invalid reference after parsing repository field", path)
			}
			return ref, nil
		}
		// If parsing repoStr fails, continue to assemble from parts (fallthrough)
		debug.Printf("Parsing repoStr ('%s') failed: %v. Will attempt assembly.", repoStr, err)
	}

	// If repoStr didn't contain tag/digest or parsing failed, try parsing the assembled string
	if fullRefStr != repoStr { // Only parse assembled if it's different from repoStr
		ref, err := ParseImageReference(fullRefStr, d.context.Strict) // Pass strict mode
		if err != nil {
			// If parsing assembled string fails, try adding global registry if available
			if d.context.GlobalRegistry != "" && regStr == "" { // Only apply if no explicit registry
				refWithGlobal := &Reference{
					Registry:   d.context.GlobalRegistry,
					Repository: repoStr, // Use original repo string
					Tag:        tagStr,  // Use explicit tag/digest if present
					Digest:     digestStr,
					Path:       copyPath(path),
					Original:   fmt.Sprintf("repository=%s,tag=%s,digest=%s (global registry applied)", repoStr, tagStr, digestStr),
				}
				NormalizeImageReference(refWithGlobal) // Normalize before validation
				if IsValidImageReference(refWithGlobal) {
					refWithGlobal.Detected = true
					return refWithGlobal, nil
				}
				// Global registry didn't help, fall through to return original assembly error
			}
			// Return original assembled parse error
			return nil, fmt.Errorf("failed to parse assembled reference '%s' from map at path %v: %w", fullRefStr, path, err)
		}

		// Parsed successfully from assembled string
		ref.Path = copyPath(path)
		ref.Original = fmt.Sprintf("repository=%s,registry=%s,tag=%s,digest=%s (assembled from map)", repoStr, regStr, tagStr, digestStr)
		ref.Detected = true
		NormalizeImageReference(ref) // Final normalization
		if !IsValidImageReference(ref) {
			return nil, fmt.Errorf("invalid map structure at path %v resulted in invalid reference after assembly and normalization", path)
		}
		return ref, nil
	} else {
		// Assembled string is the same as repoStr, and parsing repoStr failed earlier (or it had no tag/digest)
		// This implies either an invalid repoStr or a repoStr without tag/digest.
		// We need to rely on normalization to add the default tag if applicable.
		ref, err := ParseImageReference(repoStr, d.context.Strict) // Parse repoStr again, rely on normalization
		if err != nil {
			// If parsing repoStr fails, try adding global registry if available
			if d.context.GlobalRegistry != "" {
				refWithGlobal := &Reference{
					Registry:   d.context.GlobalRegistry,
					Repository: repoStr,
					Tag:        tagStr,    // Should be empty here unless explicitly set
					Digest:     digestStr, // Should be empty here unless explicitly set
					Path:       copyPath(path),
					Original:   fmt.Sprintf("repository=%s,tag=%s,digest=%s (global registry applied)", repoStr, tagStr, digestStr),
				}
				NormalizeImageReference(refWithGlobal)
				if IsValidImageReference(refWithGlobal) {
					refWithGlobal.Detected = true
					return refWithGlobal, nil
				}
			}
			// Return original parse error if global registry didn't help or wasn't present
			return nil, fmt.Errorf("failed to parse repository '%s' as image reference (and global registry didn't apply/help) at path %v: %w", repoStr, path, err)
		}

		// Parsed successfully from repoStr potentially combined with tag/digest
		ref.Path = copyPath(path)
		ref.Original = fmt.Sprintf("repository=%s,tag=%s,digest=%s (parsed from repository)", repoStr, tagStr, digestStr)
		ref.Detected = true

		// Re-apply explicit tag/digest if they weren't part of the parsed repoStr AND ref doesn't have one yet
		if tagStr != "" && ref.Tag == "" && ref.Digest == "" {
			if !isValidTag(tagStr) {
				return nil, fmt.Errorf("invalid explicit tag '%s' conflicts with parsed ref from repo at path %v", tagStr, path)
			}
			ref.Tag = tagStr
		} else if digestStr != "" && ref.Digest == "" && ref.Tag == "" {
			if !isValidDigest(digestStr) {
				return nil, fmt.Errorf("invalid explicit digest '%s' conflicts with parsed ref from repo at path %v", digestStr, path)
			}
			ref.Digest = digestStr
		} else if (tagStr != "" && ref.Digest != "") || (digestStr != "" && ref.Tag != "") {
			// Handle conflict introduced by explicit tag/digest after parsing repoStr
			return nil, fmt.Errorf("%w: map at path %v has conflicting tag/digest after parsing repository field",
				ErrTagAndDigestPresent, path)
		}

		// Final normalization needed after potentially adding explicit tag/digest
		NormalizeImageReference(ref)
		if !IsValidImageReference(ref) {
			return nil, fmt.Errorf("map at path %v is invalid after normalization (parsed from repository)", path)
		}

		return ref, nil
	}
}

// tryExtractImageFromMap checks if a map conforms to a known image structure.
// Returns:
// - *DetectedImage: The detected image if valid, nil otherwise
// - bool: true if the map matches image structure (even if invalid), false otherwise
// - error: Any validation errors encountered
func (d *Detector) tryExtractImageFromMap(m map[string]interface{}, path []string) (*DetectedImage, bool, error) {
	debug.FunctionEnter("tryExtractImageFromMap")
	defer debug.FunctionExit("tryExtractImageFromMap")
	debug.Printf("Path='%v', Map=%v", path, m)

	// Validate basic structure
	repoStr, regStr, tagStr, digestStr, isImageMap, err := d.validateMapStructure(m, path)
	if !isImageMap {
		return nil, false, err
	}

	// Check for templates
	if err := d.checkForTemplates(repoStr, regStr, tagStr, digestStr); err != nil {
		return nil, true, err
	}

	// Handle empty repository in strict mode
	if repoStr == "" {
		if d.context.Strict {
			return nil, true, fmt.Errorf("image map validation failed: repository cannot be empty at path %v", path)
		}
		return nil, true, nil
	}

	// Create and validate reference
	ref, err := d.createImageReference(repoStr, regStr, tagStr, digestStr, path)
	if err != nil {
		if d.context.Strict {
			return nil, true, err
		}
		return nil, true, nil
	}

	// Create DetectedImage
	detected := &DetectedImage{
		Reference:      ref,
		Path:           copyPath(path),
		Pattern:        "map",
		Original:       m,
		OriginalFormat: "map",
	}

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

	ref, err := ParseImageReference(s, d.context.Strict) // Pass strict mode
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

// isImagePath checks if a path is a known image path based on common patterns.
func isImagePath(path []string) bool {
	if len(path) == 0 {
		return false
	}

	lastSegment := path[len(path)-1]

	// Check if the last segment ends with "image" (case-insensitive)
	if strings.HasSuffix(strings.ToLower(lastSegment), "image") {
		// Existing checks for common preceding elements can remain if needed,
		// but the primary check is now suffix-based.
		if len(path) > 1 {
			// Example: spec.template.spec.containers[0].image
			// Example: jobTemplate.spec.template.spec.containers[0].image
			// Heuristics based on array index before the image field
			if len(path) > 2 {
				if (path[len(path)-3] == "containers" || path[len(path)-3] == "initContainers") && strings.HasPrefix(path[len(path)-2], "[") && strings.HasSuffix(path[len(path)-2], "]") {
					return true
				}
				if len(path) > 5 && path[len(path)-6] == "jobTemplate" && path[len(path)-3] == "containers" && strings.HasPrefix(path[len(path)-2], "[") && strings.HasSuffix(path[len(path)-2], "]") {
					return true
				}
				if len(path) > 5 && path[len(path)-6] == "jobTemplate" && path[len(path)-3] == "initContainers" && strings.HasPrefix(path[len(path)-2], "[") && strings.HasSuffix(path[len(path)-2], "]") {
					return true
				}
			}
			// Simpler check: Is the path segment *before* the image key an array index?
			if strings.HasPrefix(path[len(path)-2], "[") && strings.HasSuffix(path[len(path)-2], "]") {
				return true
			}
		}
		// Allow if the key ending in 'image' is the only element or if preceding element isn't an index.
		return true
	}

	// TODO: Add more sophisticated path pattern matching if needed.

	return false
}
