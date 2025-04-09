// Package image provides core functionality for detecting and manipulating container image references within Helm charts.
package image

import (
	"fmt"
	"strings"

	"errors"

	"github.com/lalbers/irr/pkg/debug"
	log "github.com/lalbers/irr/pkg/log"
)

// Detector handles the discovery of image references within chart values.
//
// NOTE: The accuracy of image detection heavily relies on the pkg/image/parser module.
// Recent simplification of createImageReference did not resolve test failures, as the
// root cause lies within the parser's handling of normalization and specific error cases.
// See TODO.md Phase 3 for details on the parser issues.
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

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage
	var err error

	switch v := values.(type) {
	case map[string]interface{}:
		detectedImages, unsupportedMatches, err = d.processMapValue(v, path)
	case []interface{}:
		detectedImages, unsupportedMatches, err = d.processSliceValue(v, path)
	case string:
		detectedImages, unsupportedMatches, err = d.processStringValue(v, path)
	default:
		// Non-mappable types are ignored, return empty slices and nil error
		return nil, nil, nil
	}

	// Handle potential error from processing functions
	if err != nil {
		return nil, nil, err
	}

	// Post-detection Filtering for Non-Strict Mode
	if !d.context.Strict && (len(d.context.SourceRegistries) > 0 || len(d.context.ExcludeRegistries) > 0) {
		debug.Printf("Applying post-detection filtering (non-strict mode) to %d images for path %v", len(detectedImages), path)
		filteredDetected := make([]DetectedImage, 0, len(detectedImages))
		for _, detected := range detectedImages {
			// Normalize before checking source registry (important for docker.io vs index.docker.io)
			NormalizeImageReference(detected.Reference)
			if IsSourceRegistry(detected.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
				filteredDetected = append(filteredDetected, detected)
			} else {
				debug.Printf("Filtering out non-source/excluded image (non-strict): %s at path %v", detected.Reference.String(), detected.Path)
				// Optionally, add to unsupportedMatches if we want to report these in non-strict mode too
				// unsupportedMatches = append(unsupportedMatches, UnsupportedImage{...})
			}
		}
		detectedImages = filteredDetected // Replace with filtered list
		debug.Printf("Finished post-detection filtering. %d images remain.", len(detectedImages))
	}

	return detectedImages, unsupportedMatches, nil
}

// processMapValue handles detection of images in map values
func (d *Detector) processMapValue(v map[string]interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	debug.Println("Processing map")
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// First, try to detect an image map at the current level
	detectedImage, isImage, err := d.tryExtractImageFromMap(v, path)
	if isImage {
		return d.handleImageMap(detectedImage, isImage, err, path)
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

// handleImageMap processes the result of tryExtractImageFromMap.
func (d *Detector) handleImageMap(detectedImage *DetectedImage, isPotentialMap bool, err error, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	if err != nil {
		debug.Printf("Handling error from map processing at path %v: %v", path, err)
		// Determine the type of unsupported image based on the error
		var unsupportedType UnsupportedType
		if errors.Is(err, ErrTemplateVariableDetected) {
			unsupportedType = UnsupportedTypeTemplateMap // Use specific code for map templates
		} else if errors.Is(err, ErrTagAndDigestPresent) {
			unsupportedType = UnsupportedTypeMapTagAndDigest
		} else {
			unsupportedType = UnsupportedTypeMapError // General map error
		}

		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     unsupportedType,
			Error:    err, // Pass the original error (already context-rich)
		})
		return detectedImages, unsupportedMatches, nil
	}

	// If no error, but it looked like a map, handle valid detection or skipped map
	if isPotentialMap {
		if detectedImage != nil {
			// Valid image map detected
			NormalizeImageReference(detectedImage.Reference)
			if IsSourceRegistry(detectedImage.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
				detectedImages = append(detectedImages, *detectedImage)
			} else {
				// Valid map, but not a source registry
				unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
					Location: path,
					Type:     UnsupportedTypeNonSourceImage,
					Error:    fmt.Errorf("map at path %v is not from a configured source registry", path),
				})
			}
		} else {
			// It looked like a map structure, but validation failed without specific error (e.g., empty repo in non-strict)
			debug.Printf("Map structure at path %v was skipped during validation (e.g., empty repo non-strict)", path)
			// No unsupported match needed here, it was just skipped.
		}
	} // else: It wasn't even a potential map, nothing to do.

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
	log.Debugf("[DEBUG irr DETECT STRING] Processing string value at path %s: %q\n", pathToString(path), v)
	log.Debugf("[DEBUG irr DETECT STRING] Path: %v, Value: '%s', Strict Context: %v\n", path, v, d.context.Strict)

	var detectedImages []DetectedImage

	isKnownImagePath := isImagePath(path)

	if d.context.Strict {
		// Delegate strict processing entirely to the helper function
		return d.processStringValueStrict(v, path, isKnownImagePath)
	}

	// Non-strict mode processing: Attempt to parse any string, ignore templates gracefully.
	imgRef, err := d.tryExtractImageFromString(v, path) // tryExtractImageFromString handles non-strict template skipping
	if err != nil {
		// In non-strict mode, any error from tryExtractImageFromString (template or parse error)
		// means we should just skip this string value.
		if errors.Is(err, ErrSkippedTemplateDetection) {
			log.Debugf("[DEBUG irr DETECT STRING SKIP] Skipping template value (non-strict) at path %s: %q\n", pathToString(path), v)
		} else {
			log.Debugf("[DEBUG irr DETECT STRING SKIP] Skipping unparseable value (non-strict) at path %s: %q, Error: %v\n", pathToString(path), v, err)
		}
		// Return nil slices and nil error for skips in non-strict mode.
		return nil, nil, nil
	}

	// If err is nil, imgRef should be non-nil (unless tryExtractImageFromString logic changes).
	if imgRef != nil {
		// In non-strict mode, always add the detected image. Filtering happens later.
		detectedImages = append(detectedImages, *imgRef)
		log.Debugf("[DEBUG irr DETECT STRING ADD] Detected image (non-strict) at path %s: %q\n", pathToString(path), v)
	} else {
		// This case should ideally not happen if err is nil, but log if it does.
		log.Warnf("[DEBUG irr DETECT STRING WARN] tryExtractImageFromString returned nil error and nil imgRef (non-strict) at path %s: %q", pathToString(path), v)
	}

	// Always return nil unsupportedMatches and nil error for non-strict string processing success/skip.
	return detectedImages, nil, nil
}

// processStringValueStrict handles string processing in strict mode
func (d *Detector) processStringValueStrict(v string, path []string, isKnownImagePath bool) ([]DetectedImage, []UnsupportedImage, error) {
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// 1. Check if path is known (Templates are checked by tryExtractImageFromString now)
	if !isKnownImagePath {
		return detectedImages, unsupportedMatches, nil
	}

	// 2. Parse and validate
	imgRefDetected, err := d.tryExtractImageFromString(v, path)
	if err != nil {
		var unsupportedType UnsupportedType
		var errMsg string

		// Check the specific error type returned by tryExtractImageFromString
		if errors.Is(err, ErrTemplateVariableDetected) {
			unsupportedType = UnsupportedTypeTemplateString // Correct type for string templates
			errMsg = fmt.Sprintf("strict mode: template variable detected in string at path %v", path)
		} else {
			// Assume other errors are parsing errors
			unsupportedType = UnsupportedTypeStringParseError
			errMsg = fmt.Sprintf("strict mode: string at known image path %v failed to parse", path)
		}

		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     unsupportedType,
			Error:    fmt.Errorf("%s", errMsg), // Use %s to format the message safely
		})
		return detectedImages, unsupportedMatches, nil
	}

	// If err is nil, proceed with source registry check or handle unexpected nil ref
	if imgRefDetected != nil {
		// Successfully parsed
		NormalizeImageReference(imgRefDetected.Reference)
		if IsSourceRegistry(imgRefDetected.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
			detectedImages = append(detectedImages, *imgRefDetected)
		} else {
			// Parsed correctly, but not a source registry
			unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
				Location: path,
				Type:     UnsupportedTypeNonSourceImage,
				Error:    fmt.Errorf("strict mode: string at path %v is not from a configured source registry", path),
			})
		}
	} else {
		// Handle the case where err is nil but imgRefDetected is also nil
		// This happens if tryExtractImageFromString heuristic skips the parse (e.g., "invalid-string")
		// In strict mode, if this happened at a known image path, it's an error.
		if isKnownImagePath {
			debug.Printf("Strict mode: String at known image path %v was skipped by heuristic or returned nil ref unexpectedly.", path)
			unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
				Location: path,
				Type:     UnsupportedTypeStringParseError, // Treat heuristic skip at known path as parse error
				Error:    fmt.Errorf("strict mode: string at known image path %v was skipped (likely invalid format)", path),
			})
		}
		// If not a known image path, returning nil, nil implicitly skips it, which is fine.
	}

	return detectedImages, unsupportedMatches, nil
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

	// Type assertions and handling templates
	repoStr, repoIsString := repoVal.(string)
	if !repoIsString {
		// If not a string, it can't be valid unless it's a template detected elsewhere.
		// We don't check for templates here, as it's not a string.
		return "", "", "", "", false, nil
	}
	// Check if the string repoStr contains a template
	if containsTemplate(repoStr) {
		return repoStr, "", "", "", true, ErrTemplateVariableDetected // Return extracted repoStr with error
	}

	// Process Registry (optional)
	regStr := ""
	if regOk {
		var regIsString bool
		regStr, regIsString = regVal.(string)
		if !regIsString {
			// Not a string - invalid structure
			debug.Printf("[validateMapStructure] Invalid non-string registry value at path %v.", path)
			return "", "", "", "", false, nil
		} else if containsTemplate(regStr) { // Check string value for template
			return repoStr, regStr, "", "", true, ErrTemplateVariableDetected // Return extracted values with error
		}
	}

	// Process Tag (optional)
	tagStr := ""
	if tagOk {
		var tagIsString bool
		tagStr, tagIsString = tagVal.(string)
		if !tagIsString {
			// Not a string - invalid structure
			debug.Printf("[validateMapStructure] Invalid non-string tag value at path %v.", path)
			return "", "", "", "", false, nil
		} else if containsTemplate(tagStr) { // Check string value for template
			return repoStr, regStr, tagStr, "", true, ErrTemplateVariableDetected // Return extracted values with error
		}
	}

	// Process Digest (optional)
	digestStr := ""
	if digestOk {
		var digestIsString bool
		digestStr, digestIsString = digestVal.(string)
		if !digestIsString {
			// Not a string - invalid structure
			debug.Printf("[validateMapStructure] Invalid non-string digest value at path %v.", path)
			return "", "", "", "", false, nil
		} else if containsTemplate(digestStr) { // Check string value for template
			return repoStr, regStr, tagStr, digestStr, true, ErrTemplateVariableDetected // Return extracted values with error
		}
	}

	return repoStr, regStr, tagStr, digestStr, true, nil
}

// createImageReference attempts to create a valid image Reference object from components extracted from a map.
// It prioritizes explicit fields (registry, tag, digest) over information potentially embedded in the repository string.
func (d *Detector) createImageReference(repoStr, regStr, tagStr, digestStr string, path []string) (*Reference, error) {
	debug.FunctionEnter("Detector.createImageReference")
	debug.Printf("Path: %v, Repo: %q", path, repoStr)
	debug.Printf("[DEBUG createImageReference] Inputs: Repo=%q, Reg=%q, Tag=%q, Digest=%q", repoStr, regStr, tagStr, digestStr)
	if repoStr == "" {
		debug.Printf("[DEBUG createImageReference] Repository string is empty.")
		return nil, fmt.Errorf("repository field is mandatory but was empty at path %v", path)
	}

	// Basic validation: cannot have both tag and digest in the map structure itself.
	if tagStr != "" && digestStr != "" {
		return nil, fmt.Errorf("%w: map at path %v has both tag ('%s') and digest ('%s')",
			ErrTagAndDigestPresent, path, tagStr, digestStr)
	}

	// Assemble the image string from parts.
	builder := strings.Builder{}
	registryApplied := "none"

	if regStr != "" {
		// Explicit registry always wins.
		builder.WriteString(regStr)
		builder.WriteByte('/')
		registryApplied = regStr + " (explicit)"
		debug.Printf("Using explicit registry: %s", regStr)
	} else {
		// No explicit registry. Check if repoStr contains a potential registry prefix.
		hasRegistryPrefix := false
		if repoParts := strings.SplitN(repoStr, "/", 2); len(repoParts) > 1 {
			firstPart := repoParts[0]
			if strings.ContainsAny(firstPart, ".:") || firstPart == "localhost" {
				hasRegistryPrefix = true
				debug.Printf("Detected potential registry prefix ('%s') in repoStr ('%s'). Skipping global registry.", firstPart, repoStr)
				registryApplied = firstPart + " (in repoStr)"
			}
		}

		if !hasRegistryPrefix && d.context.GlobalRegistry != "" {
			// Use global registry only if repoStr doesn't have its own prefix.
			builder.WriteString(d.context.GlobalRegistry)
			builder.WriteByte('/')
			registryApplied = d.context.GlobalRegistry + " (global)"
			debug.Printf("Applying global registry: %s", d.context.GlobalRegistry)
		} else if hasRegistryPrefix {
			// repoStr contains registry, do nothing here for the registry part, it's part of repoStr.
			debug.Printf("Using registry implicitly included in repoStr: %s", repoStr)
		} else {
			// No explicit registry, no global registry, repoStr has no prefix.
			// The ParseImageReference call later will handle normalization (e.g., adding docker.io/library).
			debug.Printf("No explicit or global registry provided, and repoStr ('%s') has no prefix.", repoStr)
		}
	}

	// Add the repository string itself.
	builder.WriteString(repoStr)

	// Add tag or digest if present.
	if tagStr != "" {
		builder.WriteByte(':')
		builder.WriteString(tagStr)
	} else if digestStr != "" {
		builder.WriteByte('@')
		builder.WriteString(digestStr)
	}

	candidateStr := builder.String()
	debug.Printf("Assembled candidate string: '%s' (Registry Applied: %s)", candidateStr, registryApplied)

	// Parse the constructed candidate string using the canonical parser.
	ref, err := ParseImageReference(candidateStr)
	if err != nil {
		debug.Printf("Error parsing assembled reference '%s' at path %v: %v", candidateStr, path, err)
		return nil, fmt.Errorf("failed to parse assembled reference '%s' from path %v: %w", candidateStr, path, err)
	}

	// Ensure the reference is marked as detected from the structure
	ref.Detected = true

	return ref, nil
}

// tryExtractImageFromMap checks if a map conforms to a known image structure.
// Returns:
// - *DetectedImage: The detected image if valid, nil otherwise
// - bool: true if the map matches image structure (even if invalid), false otherwise
// - error: Any validation errors encountered
func (d *Detector) tryExtractImageFromMap(m map[string]interface{}, path []string) (*DetectedImage, bool, error) {
	debug.FunctionEnter("Detector.tryExtractImageFromMap")
	debug.DumpValue("Input map", m)

	// Validate basic structure
	repoStr, regStr, tagStr, digestStr, isImageMap, err := d.validateMapStructure(m, path)

	// If validation returned an error (like template detected) or it's not an image map structure,
	// return immediately with the results from validateMapStructure.
	if err != nil || !isImageMap {
		debug.Printf("[tryExtractImageFromMap] Validation failed or template detected. isImageMap: %t, err: %v", isImageMap, err)
		return nil, isImageMap, err
	}

	// Handle empty repository in strict mode
	if repoStr == "" {
		if d.context.Strict {
			return nil, true, fmt.Errorf("image map validation failed: repository cannot be empty at path %v", path)
		}
		return nil, true, nil
	}

	// 4. Construct the image reference using createImageReference
	debug.Printf("[DEBUG tryExtractImageFromMap] Calling createImageReference with: Repo=%q, Reg=%q, Tag=%q, Digest=%q, Path=%v", repoStr, regStr, tagStr, digestStr, path)
	imgRef, err := d.createImageReference(repoStr, regStr, tagStr, digestStr, path)

	// Log the result of createImageReference
	if err != nil {
		debug.Printf("[DEBUG tryExtractImageFromMap] createImageReference returned error: %v", err)
	} else if imgRef == nil {
		debug.Printf("[DEBUG tryExtractImageFromMap] createImageReference returned nil image ref and nil error")
	} else {
		debug.Printf("[DEBUG tryExtractImageFromMap] createImageReference returned image ref: %s", imgRef.String())
	}

	if err != nil {
		// Propagate error from createImageReference (e.g., parse error)
		debug.Printf("[DEBUG tryExtractImageFromMap] Returning error from createImageReference: %v", err)
		// Return isImageMap=true because the structure *looked* like an image map, even if parsing failed.
		// The error indicates the problem.
		return nil, true, fmt.Errorf("map structure at path %v resembles an image, but failed validation: %w", path, err)
	}

	// 5. Construct and return the DetectedImage object if validation passes
	detected := &DetectedImage{
		Reference: imgRef,
		Path:      path,
		Pattern:   PatternMap, // Set pattern to map
		Original:  m,          // Store the original map
	}
	debug.Printf("[DEBUG tryExtractImageFromMap] Successfully detected map-based image. Returning: %+v", detected.Reference)
	debug.FunctionExit("Detector.tryExtractImageFromMap (Returning image)")
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
		// Always return the specific error if a template is found.
		return nil, ErrTemplateVariableDetected
	}

	// Heuristic: If the string doesn't contain typical image separators, it's unlikely an image ref.
	if !strings.ContainsAny(s, "/:@") {
		debug.Printf("String '%s' lacks image separators (/ : @), skipping parse attempt.", s)
		// Return nil, nil to silently skip in non-strict, allows strict checks later if needed.
		// If strict mode required parsing *any* string at known paths, this logic would need adjustment,
		// but current strict logic relies on isImagePath, so skipping non-image-like strings here is okay.
		return nil, nil
	}

	// DEBUG: Log input to ParseImageReference
	debug.Printf("Calling ParseImageReference with: %s", s)

	ref, err := ParseImageReference(s)

	// DEBUG: Log output from ParseImageReference
	debug.Printf("ParseImageReference returned: err=%v", err)
	// Use %+v to see struct details if ref is not nil
	debug.Printf("ParseImageReference returned: ref=%+v", ref)

	if err != nil {
		// Restore original debug message and error format
		debug.Printf("ParseImageReference err: %v", err)
		return nil, fmt.Errorf("invalid image string format: %w", err)
	}

	// Parsed successfully
	ref.Path = copyPath(path) // Store path where string was detected
	ref.Detected = true
	ref.Original = s // Restore setting the original string

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
		return true // Simplified: Any key ending in 'image' is considered a potential path
	}

	// In strict mode, also consider standard map keys as known paths
	switch lastSegment {
	case "repository", "registry", "tag", "digest":
		// We only consider these known if they are part of a potential parent map.
		// A simple check: is the path length > 1?
		if len(path) > 1 {
			return true
		}
	}

	// TODO: Add more sophisticated path pattern matching if needed.

	return false
}
