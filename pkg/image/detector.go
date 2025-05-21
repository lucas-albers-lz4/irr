// Package image provides core functionality for detecting and manipulating container image references within Helm charts.
package image

import (
	"fmt"
	"strings"

	"errors"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

// Add these error declarations at the top of the file, after the import section
var (
	// ErrNotImageString indicates that a string doesn't have the characteristics of an image reference
	ErrNotImageString = errors.New("string does not appear to be an image reference")
)

const (
	// MaxComponents is the maximum number of components to split an image path into
	MaxComponents = 2
	// LocalhostRegistry is the constant used for identifying localhost registry references
	LocalhostRegistry = "localhost" // Constant for localhost registry references
)

// MapStructureResult holds the result of validating an image map structure.
// It contains the parsed components of an image reference and validation status.
type MapStructureResult struct {
	// Repository is the repository component of the image reference
	Repository string
	// Registry is the registry component of the image reference (if present)
	Registry string
	// Tag is the tag component of the image reference (if present)
	Tag string
	// Digest is the digest component of the image reference (if present)
	Digest string
	// IsImageMap indicates whether the map structure matches an image pattern
	IsImageMap bool
	// TemplateErr contains any template-related error encountered during validation
	TemplateErr error
}

// Detector handles the discovery of image references within chart values.
// It implements a recursive traversal strategy to find image references in
// complex nested structures like Helm chart values.yaml files.
//
// The detector can handle:
// - Map-based image references (registry/repository/tag)
// - String-based image references (e.g., "nginx:latest")
// - Nested structures with arrays and maps
// - Template variables in image references (when TemplateMode is enabled)
// - Filtering by source registry
//
// NOTE: The accuracy of image detection heavily relies on the pkg/image/parser module.
// Recent simplification of createImageReference did not resolve test failures, as the
// root cause lies within the parser's handling of normalization and specific error cases.
// See TODO.md Phase 3 for details on the parser issues.
type Detector struct {
	context            *DetectionContext
	shouldProcessValue func(string) bool // Function that determines if a string value should be processed
}

// NewDetector creates a new Detector with the specified detection context.
// The context controls filtering behavior, strict mode, and template handling.
func NewDetector(context *DetectionContext) *Detector {
	log.Debug("NewDetector", "context", context)

	// For additional safety, create a new DetectionContext if none is provided
	// This ensures we never have a nil context
	detector := &Detector{
		// Default behavior: process all string values (can be overridden by caller)
		shouldProcessValue: func(s string) bool {
			// Use the parameter to avoid unused parameter warning
			// Always return true regardless of string value
			return s != "" || s == ""
		},
	}

	// Use the provided context or create a default one
	if context == nil {
		context = &DetectionContext{
			SourceRegistries:  []string{},
			ExcludeRegistries: []string{},
		}
		log.Debug("NewDetector", "created default DetectionContext", *context)
	} else {
		// Initialize fields if nil
		if context.SourceRegistries == nil {
			context.SourceRegistries = []string{}
			log.Debug("NewDetector", "initializing nil SourceRegistries with empty slice")
		}

		if context.ExcludeRegistries == nil {
			context.ExcludeRegistries = []string{}
			log.Debug("NewDetector", "initializing nil ExcludeRegistries with empty slice")
		}
	}

	// Store the context
	detector.context = context

	log.Debug("NewDetector", "initialized Detector with context", *detector.context)
	return detector
}

// DetectImages recursively traverses the values structure to find image references.
// It processes maps, slices, and strings differently to detect image references
// and returns both detected images and unsupported structures.
//
// Parameters:
//   - values: The structure to traverse (typically a map[string]interface{} from parsed YAML)
//   - path: The current path in the structure (used for reporting and validation)
//
// Returns:
//   - []DetectedImage: Successfully detected and validated image references
//   - []UnsupportedImage: Image-like structures that couldn't be processed
//   - error: Any error encountered during processing
func (d *Detector) DetectImages(values interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	log.Debug("Detector.DetectImages", "path", path)

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

	log.Debug("Detector.DetectImages", "Input values", values)
	log.Debug("Detector.DetectImages", "Current path", path)
	log.Debug("Detector.DetectImages", "Context", d.context)
	log.Debug("Detector.DetectImages", "DETECTOR ENTRY", "Path", path, "Type", values)

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
		log.Debug("Applying post-detection filtering (non-strict mode) to", len(detectedImages), "images for path", path)
		filteredDetected := make([]DetectedImage, 0, len(detectedImages))
		for _, detected := range detectedImages {
			// Add a nil check before attempting to normalize the Reference
			if detected.Reference == nil {
				log.Debug("Skipping nil Reference at path", detected.Path)
				continue
			}
			// Normalize before checking source registry (important for docker.io vs index.docker.io)
			NormalizeImageReference(detected.Reference)
			if IsSourceRegistry(detected.Reference, d.context.SourceRegistries, d.context.ExcludeRegistries) {
				filteredDetected = append(filteredDetected, detected)
			} else {
				log.Debug("Filtering out non-source/excluded image (non-strict):", detected.Reference.String(), "at path", detected.Path)
				// Optionally, add to unsupportedMatches if we want to report these in non-strict mode too
				// unsupportedMatches = append(unsupportedMatches, UnsupportedImage{...})
			}
		}
		detectedImages = filteredDetected // Replace with filtered list
		log.Debug("Finished post-detection filtering.", len(detectedImages), "images remain.")
	}

	return detectedImages, unsupportedMatches, nil
}

// processMapValue handles detection of images in map values
func (d *Detector) processMapValue(v map[string]interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	log.Debug("Processing map")

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// First, try to detect an image map at the current level
	detectedImage, isImage, err := d.tryExtractImageFromMap(v, path)
	if isImage {
		return d.handleImageMap(detectedImage, isImage, err, path)
	}

	// If not an image map, recurse into values
	log.Debug("Structure at path", PathToString(path), "did not match image map pattern, recursing into values.")
	for key, val := range v {
		newPath := append(append([]string{}, path...), key)
		detected, unsupported, err := d.DetectImages(val, newPath)
		if err != nil {
			log.Error("Error processing path", newPath, ":", err)
			return nil, nil, fmt.Errorf("error processing path %v: %w", newPath, err)
		}
		detectedImages = append(detectedImages, detected...)
		if len(unsupported) > 0 {
			log.Debug("Appending", len(unsupported), "items from key", key)
			for i, item := range unsupported {
				log.Debug("Appending item", i, "Path:", item.Location, "Type:", item.Type, "Error:", item.Error)
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

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

	if err != nil {
		log.Debug("Handling error from map processing at path", path, "error", err)
		// Determine the type of unsupported image based on the error
		var unsupportedType UnsupportedType

		switch {
		case errors.Is(err, ErrTemplateVariableDetected):
			unsupportedType = UnsupportedTypeTemplateMap // Use specific code for map templates
		case errors.Is(err, ErrTagAndDigestPresent):
			unsupportedType = UnsupportedTypeMapTagAndDigest
		default:
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
			if detectedImage.Reference == nil {
				log.Debug("Skipping nil Reference for detectedImage at path", path)
				return detectedImages, unsupportedMatches, nil
			}
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
			log.Debug("Map structure at path", path, "was skipped during validation (e.g., empty repo non-strict)")
			// No unsupported match needed here, it was just skipped.
		}
	} // else: It wasn't even a potential map, nothing to do.

	return detectedImages, unsupportedMatches, nil
}

// processSliceValue handles detection of images in slice/array values
func (d *Detector) processSliceValue(v []interface{}, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	log.Debug("Processing slice/array")

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	for i, item := range v {
		itemPath := append(append([]string{}, path...), fmt.Sprintf("[%d]", i))
		log.Debug("Recursively processing slice item", i, "at path", itemPath)
		detectedInItem, unsupportedInItem, err := d.DetectImages(item, itemPath)
		if err != nil {
			log.Error("Error processing slice item at path", itemPath, ":", err)
			return nil, nil, fmt.Errorf("error processing slice item %d at path %s: %w", i, path, err)
		}
		detectedImages = append(detectedImages, detectedInItem...)
		if len(unsupportedInItem) > 0 {
			log.Debug("Appending", len(unsupportedInItem), "items from index", i)
			for j, item := range unsupportedInItem {
				log.Debug("Appending item", j, "Path:", item.Location, "Type:", item.Type, "Error:", item.Error)
			}
		}
		unsupportedMatches = append(unsupportedMatches, unsupportedInItem...)
	}
	return detectedImages, unsupportedMatches, nil
}

// processStringValue handles detection of images in string values
func (d *Detector) processStringValue(v string, path []string) ([]DetectedImage, []UnsupportedImage, error) {
	log.Debug("[DEBUG irr DETECT STRING] Processing string value", "path", PathToString(path), "value", v)

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

	log.Debug("[DEBUG irr DETECT STRING] Path", path, "Value", v, "Strict Context", d.context.Strict)

	var detectedImages []DetectedImage

	isKnownImagePath := isImagePath(path)

	// IMPORTANT: Added safeguard for nil shouldProcessValue - if this is triggering,
	// the Detector struct is missing initialization of the shouldProcessValue field/function
	if d.shouldProcessValue != nil && !d.shouldProcessValue(v) {
		log.Debug("Skipping string value due to shouldProcessValue filter", "value", v)
		return nil, nil, nil
	} else if d.shouldProcessValue == nil {
		// If shouldProcessValue is nil, log a warning and continue processing
		log.Warn("shouldProcessValue function is nil in Detector, skipping filter check at path %v", path)
	}

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
			log.Debug("[DEBUG irr DETECT STRING SKIP] Skipping template value (non-strict)", "path", PathToString(path), "value", v)
		} else {
			log.Debug("[DEBUG irr DETECT STRING SKIP] Skipping unparseable value (non-strict)", "path", PathToString(path), "value", v, "error", err)
		}
		// Return nil slices and nil error for skips in non-strict mode.
		return nil, nil, nil
	}

	// If err is nil, imgRef should be non-nil (unless tryExtractImageFromString logic changes).
	if imgRef != nil {
		// In non-strict mode, always add the detected image. Filtering happens later.
		detectedImages = append(detectedImages, *imgRef)
		log.Debug("[DEBUG irr DETECT STRING ADD] Detected image (non-strict)", "path", PathToString(path), "value", v)
	} else {
		// This case should ideally not happen if err is nil, but log if it does.
		log.Warn("[DEBUG irr DETECT STRING WARN] tryExtractImageFromString returned nil error and nil imgRef (non-strict)", "path", PathToString(path), "value", v)
	}

	// Always return nil unsupportedMatches and nil error for non-strict string processing success/skip.
	return detectedImages, nil, nil
}

// processStringValueStrict handles string processing in strict mode
func (d *Detector) processStringValueStrict(v string, path []string, isKnownImagePath bool) ([]DetectedImage, []UnsupportedImage, error) {
	var detectedImages []DetectedImage
	var unsupportedMatches []UnsupportedImage

	// Add nil check for context
	if d.context == nil {
		log.Error("Context is nil in Detector at path %v, this is likely a configuration error", path)
		return nil, nil, fmt.Errorf("detector context is nil (configuration error)")
	}

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
			errMsg = fmt.Sprintf("strict mode: string at known image path %v was skipped (likely invalid format)", path)
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
	} else if isKnownImagePath {
		// Handle the case where err is nil but imgRefDetected is also nil
		// This happens if tryExtractImageFromString heuristic skips the parse (e.g., "invalid-string")
		// In strict mode, if this happened at a known image path, it's an error.
		log.Debug("Strict mode: String at known image path", path, "was skipped by heuristic or returned nil ref unexpectedly.")
		unsupportedMatches = append(unsupportedMatches, UnsupportedImage{
			Location: path,
			Type:     UnsupportedTypeStringParseError, // Treat heuristic skip at known path as parse error
			Error:    fmt.Errorf("strict mode: string at known image path %v was skipped (likely invalid format)", path),
		})
		// If not a known image path, returning nil, nil implicitly skips it, which is fine.
	}

	return detectedImages, unsupportedMatches, nil
}

// validateMapStructure checks if a map has the required image structure
func (d *Detector) validateMapStructure(m map[string]interface{}, path []string) MapStructureResult {
	result := MapStructureResult{
		IsImageMap: false,
	}

	// Extract values and validate types
	repoVal, repoOk := m["repository"]
	regVal, regOk := m["registry"]
	tagVal, tagOk := m["tag"]
	digestVal, digestOk := m["digest"]

	// Check repository key presence
	if !repoOk {
		log.Debug("Path", path, "missing required 'repository' key.")
		return result
	}

	// Type assertions and handling templates
	repoStr, repoIsString := repoVal.(string)
	if !repoIsString {
		// If not a string, it can't be valid unless it's a template detected elsewhere.
		// We don't check for templates here, as it's not a string.
		return result
	}

	// Check if the string repoStr contains a template
	if containsTemplate(repoStr) {
		result.Repository = repoStr
		result.IsImageMap = true
		result.TemplateErr = ErrTemplateVariableDetected
		return result
	}

	// Process Registry (optional)
	if regOk {
		regStr, regIsString := regVal.(string)
		if !regIsString {
			// Not a string - invalid structure
			log.Debug("validateMapStructure", "Invalid non-string registry value at path", path)
			return result
		} else if containsTemplate(regStr) {
			result.Repository = repoStr
			result.Registry = regStr
			result.IsImageMap = true
			result.TemplateErr = ErrTemplateVariableDetected
			return result
		}
		result.Registry = regStr
	}

	// Process Tag (optional)
	switch {
	case tagOk:
		tagStr, tagIsString := tagVal.(string)
		if !tagIsString {
			// Not a string - invalid structure
			log.Debug("validateMapStructure", "Invalid non-string tag value at path", path)
			return result
		} else if containsTemplate(tagStr) {
			result.Repository = repoStr
			result.Tag = tagStr
			result.IsImageMap = true
			result.TemplateErr = ErrTemplateVariableDetected
			return result
		}
		result.Tag = tagStr

	case d.context != nil && d.context.ChartMetadata != nil && d.context.ChartMetadata.AppVersion != "":
		// If no tag is specified but Chart.AppVersion is available, use that instead of defaulting to "latest"
		log.Debug("No tag specified in map structure, using Chart.AppVersion", "path", path, "appVersion", d.context.ChartMetadata.AppVersion)
		result.Tag = d.context.ChartMetadata.AppVersion

	default:
		// No tag and no AppVersion, will default to latest during parsing
		log.Debug("No tag specified in map structure and no AppVersion available", "path", path)
	}

	// Process Digest (optional)
	if digestOk {
		digestStr, digestIsString := digestVal.(string)
		if !digestIsString {
			// Not a string - invalid structure
			log.Debug("validateMapStructure", "Invalid non-string digest value at path", path)
			return result
		} else if containsTemplate(digestStr) {
			result.Repository = repoStr
			result.Digest = digestStr
			result.IsImageMap = true
			result.TemplateErr = ErrTemplateVariableDetected
			return result
		}
		result.Digest = digestStr
	}

	// All validation passed
	result.Repository = repoStr
	result.IsImageMap = true
	return result
}

// createImageReference attempts to create a valid image Reference object from components extracted from a map.
// It prioritizes explicit fields (registry, tag, digest) over information potentially embedded in the repository string.
func (d *Detector) createImageReference(repoStr, regStr, tagStr, digestStr string, path []string) (*Reference, error) {
	log.Debug("Detector.createImageReference", "Path", path, "Repo", repoStr)
	log.Debug("[DEBUG createImageReference] Inputs: Repo=", repoStr, "Reg=", regStr, "Tag=", tagStr, "Digest=", digestStr)
	if repoStr == "" {
		log.Debug("[DEBUG createImageReference] Repository string is empty.")
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

	// Determine which registry to apply using a switch statement instead of if-else chain
	hasRegistryPrefix := false
	parts := strings.SplitN(repoStr, "/", MaxComponents) // Store the result
	if regStr == "" && len(parts) > 1 {
		// Check length again for safety/linter before accessing index 0
		if len(parts) > 0 {
			firstPart := parts[0]
			hasRegistryPrefix = strings.ContainsAny(firstPart, ".:") || firstPart == LocalhostRegistry
		}
	}

	switch {
	case regStr != "":
		// Case 1: Explicit registry provided
		builder.WriteString(regStr)
		builder.WriteByte('/')
		registryApplied = regStr + " (explicit)"
		log.Debug("Using explicit registry:", regStr)

	case hasRegistryPrefix:
		// Case 2: No explicit registry, but repoStr contains registry prefix
		// Don't add anything to builder here, as registry is part of repoStr
		// Reuse 'parts' if it's guaranteed to be the same split, or re-split and check
		repoParts := strings.SplitN(repoStr, "/", MaxComponents) // Re-split for clarity or reuse parts if context allows
		if len(repoParts) > 0 {                                  // Add check before accessing
			firstPart := repoParts[0]
			registryApplied = firstPart + " (in repoStr)"
			log.Debug("Detected potential registry prefix ('", firstPart, "') in repoStr ('", repoStr, "'). Skipping global registry.")
		} else {
			// Handle unexpected case where split is empty, though hasRegistryPrefix should be false then
			log.Warn("Unexpected empty split result for repoStr despite hasRegistryPrefix being true:", repoStr)
		}

	case d.context.GlobalRegistry != "":
		// Case 3: Use global registry
		builder.WriteString(d.context.GlobalRegistry)
		builder.WriteByte('/')
		registryApplied = d.context.GlobalRegistry + " (global)"
		log.Debug("Applying global registry:", d.context.GlobalRegistry)

	default:
		// Case 4: No registry information available
		log.Debug("No explicit or global registry provided, and repoStr ('", repoStr, "') has no prefix.")
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
	log.Debug("Assembled candidate string:", "'", candidateStr, "' (Registry Applied:", registryApplied, ")")

	// Get ChartMetadata from context if available
	var chartMetadata *ChartMetadata
	if d.context != nil && d.context.ChartMetadata != nil {
		chartMetadata = d.context.ChartMetadata
	}

	// Parse the constructed candidate string using the canonical parser with ChartMetadata
	ref, err := ParseImageReference(candidateStr, chartMetadata)
	if err != nil {
		log.Debug("Error parsing assembled reference", "'", candidateStr, "' at path", path, ":", err)
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
	log.Debug("Detector.tryExtractImageFromMap", "Input map", m)

	// Validate basic structure
	result := d.validateMapStructure(m, path)

	// If validation returned an error (like template detected) or it's not an image map structure,
	// return immediately with the results from validateMapStructure.
	if result.TemplateErr != nil || !result.IsImageMap {
		log.Debug("[tryExtractImageFromMap] Validation failed or template detected. isImageMap:", result.IsImageMap, "err:", result.TemplateErr)
		return nil, result.IsImageMap, result.TemplateErr
	}

	// Handle empty repository in strict mode
	if result.Repository == "" {
		if d.context.Strict {
			return nil, true, fmt.Errorf("image map validation failed: repository cannot be empty at path %v", path)
		}
		return nil, true, nil
	}

	// Handle registry prefix that might be part of the repo string
	if result.Repository != "" {
		// Check if repo string might have registry prefix (contains '/')
		if repoParts := strings.SplitN(result.Repository, "/", MaxComponents); len(repoParts) > 1 {
			// The first part looks like a registry hostname if it contains dots or ':'
			// e.g., "docker.io/nginx" or "localhost:5000/myapp"
			if strings.Contains(repoParts[0], ".") || strings.Contains(repoParts[0], ":") {
				result.Registry = repoParts[0]
				result.Repository = repoParts[1]
			}
		}
	}

	// 4. Construct the image reference using createImageReference
	log.Debug("[DEBUG tryExtractImageFromMap] Calling createImageReference with: Repo=", result.Repository, "Reg=", result.Registry, "Tag=", result.Tag, "Digest=", result.Digest, "Path=", path)
	imgRef, err := d.createImageReference(result.Repository, result.Registry, result.Tag, result.Digest, path)

	// Log the result of createImageReference
	switch {
	case err != nil:
		log.Debug("[DEBUG tryExtractImageFromMap] createImageReference returned error:", err)
	case imgRef == nil:
		log.Debug("[DEBUG tryExtractImageFromMap] createImageReference returned nil image ref and nil error")
	default:
		log.Debug("[DEBUG tryExtractImageFromMap] createImageReference returned image ref:", imgRef.String())
	}

	if err != nil {
		// Propagate error from createImageReference (e.g., parse error)
		log.Debug("[DEBUG tryExtractImageFromMap] Returning error from createImageReference:", err)
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
	log.Debug("[DEBUG tryExtractImageFromMap] Successfully detected map-based image. Returning:", detected.Reference)
	return detected, true, nil
}

// tryExtractImageFromString attempts to parse a string value as a Docker image reference.
// Returns the parsed DetectedImage or nil if it's not a valid image format.
// Returns ErrTemplateVariableDetected if a template is found.
func (d *Detector) tryExtractImageFromString(s string, path []string) (*DetectedImage, error) {
	log.Debug("tryExtractImageFromString", "Path", path, "String", s)

	// Debug ChartMetadata
	if d.context != nil {
		log.Debug("Context is NOT nil")
		if d.context.ChartMetadata != nil {
			log.Debug("ChartMetadata is available",
				"AppVersion", d.context.ChartMetadata.AppVersion,
				"Name", d.context.ChartMetadata.Name,
				"Version", d.context.ChartMetadata.Version)
		} else {
			log.Debug("ChartMetadata is NIL")
		}
	} else {
		log.Debug("Context is NIL")
	}

	// Skip template variables in normal mode
	if containsTemplate(s) {
		log.Debug("[DEBUG irr DETECT STRING SKIP] Skipping template string:", s)
		return nil, ErrTemplateVariableDetected
	}

	// Skip strings that don't look like image references
	if !strings.ContainsAny(s, "/:@") {
		log.Debug("String", s, "lacks image separators (/ : @), skipping parse attempt.")
		// Return a sentinel error instead of nil, nil
		return nil, ErrNotImageString
	}

	// DEBUG: Log input to ParseImageReference
	log.Debug("Calling ParseImageReference with:", s)

	// Get the chart metadata from context if available
	var chartMetadata *ChartMetadata
	if d.context != nil && d.context.ChartMetadata != nil {
		chartMetadata = d.context.ChartMetadata
	}

	// Pass the chart metadata to use AppVersion as fallback instead of "latest"
	ref, err := ParseImageReference(s, chartMetadata)

	// DEBUG: Log output from ParseImageReference
	log.Debug("ParseImageReference returned:", "err", err)
	// Use %+v to see struct details if ref is not nil
	log.Debug("ParseImageReference returned:", "ref", ref)

	if err != nil {
		// Restore original debug message and error format
		log.Debug("ParseImageReference err:", err)
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
	log.Debug("[DETECTOR DEBUG tryExtractImageFromString] Returning DetectedImage for path", detected.Path, "with OriginalFormat:", detected.OriginalFormat)
	log.Debug("DumpValue", "DETECTOR DEBUG tryExtractImageFromString", "DetectedImage Value", detected)

	return detected, nil
}

// containsTemplate checks if a string contains Go template syntax.
func containsTemplate(s string) bool {
	hasOpen := strings.Contains(s, "{{")
	hasClose := strings.Contains(s, "}}")
	result := hasOpen && hasClose
	log.Debug("[containsTemplate DEBUG] String:", s, "HasOpen:", hasOpen, "HasClose:", hasClose, "Result:", result)
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

// PathToString converts a path slice to a dot-separated string for logging.
func PathToString(path []string) string {
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
