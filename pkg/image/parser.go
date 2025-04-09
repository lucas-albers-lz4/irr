package image

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
	"github.com/lalbers/irr/pkg/debug"
)

// Constants for regex patterns
// Remove unused tagPattern and digestPattern

// Unused constant, removing.
// const referencePattern = `^` +
// 	`(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?` + // Registry (optional)
// 	`(?P<repository>[a-zA-Z0-9_./-]+)` + // Repository (required)
// 	`(?:(?P<separator>[:@])` + // Separator (: or @)
// 	`(?P<tagordigest>[a-zA-Z0-9_.-]+(?:[.+_-][a-zA-Z0-9_.-]+)*))?` + // Tag or Digest (optional)
// 	`$`

// Unused variable, removing.
// var compiledReferenceRegex = regexp.MustCompile(referencePattern)

const (
	// maxSplitTwo is the limit for splitting into at most two parts
	maxSplitTwo = 2
)

// ParseImageReference attempts to parse an image reference string into its components.
// It uses the official distribution library first and falls back to a custom parser
// for potentially slightly malformed strings if not in strict mode.
// In strict mode, only references strictly conforming to the distribution specification are parsed successfully.
func ParseImageReference(imageStr string, strict bool) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s', Strict mode: %t", imageStr, strict)

	if imageStr == "" {
		debug.Println("Error: Input string is empty.")
		return nil, ErrEmptyImageReference // Use canonical error
	}

	var ref Reference
	ref.Original = imageStr // Set the Original field to input string

	// Use the distribution library for robust parsing first
	parsed, err := reference.ParseNamed(imageStr)
	if err != nil {
		debug.Printf("Initial distribution parse failed for '%s': %v.", imageStr, err)
		// In strict mode, DO NOT fall back to custom parsing.
		if strict {
			debug.Printf("[STRICT MODE] Distribution parse failed, returning error immediately.")
			return nil, fmt.Errorf("strict mode: failed to parse image reference '%s': %w", imageStr, err)
		}

		// Non-strict mode: Fallback to custom parsing logic
		debug.Printf("[NON-STRICT MODE] Attempting custom parse fallback.")
		customRef, customErr := parseImageReferenceCustom(imageStr)
		if customErr != nil {
			debug.Printf("Custom parse also failed for '%s': %v", imageStr, customErr)
			return nil, fmt.Errorf("failed to parse image reference '%s': distribution error: %v, custom fallback error: %w", imageStr, err, customErr)
		}
		debug.Printf("Custom parse succeeded for '%s'. Ref: %+v", imageStr, customRef)

		// Assign Original string here for the custom path
		customRef.Original = imageStr

		// We have a potentially incomplete ref from custom parsing, now normalize and validate
		NormalizeImageReference(&customRef)
		debug.Printf("Normalized custom-parsed ref: %+v", customRef)

		// Perform validation *after* normalization for custom parse path
		if !IsValidImageReference(&customRef) {
			debug.Printf("Validation failed for normalized custom-parsed ref: %+v", customRef)
			return nil, fmt.Errorf("invalid image reference after custom parse and normalization: %s", imageStr)
		}
		debug.Printf("Successfully parsed reference after custom parse and normalization: %+v", customRef)
		return &customRef, nil
	}

	// Distribution parsing succeeded, build Reference object from parsed data
	ref = Reference{
		Registry:   reference.Domain(parsed), // Use Domain() method
		Repository: reference.Path(parsed),   // Use Path() method
		Original:   imageStr,
	}

	// Add tag or digest using type assertions
	if tagged, ok := parsed.(reference.Tagged); ok {
		ref.Tag = tagged.Tag()
	} else if digested, ok := parsed.(reference.Digested); ok {
		ref.Digest = digested.Digest().String()
	}

	NormalizeImageReference(&ref) // Apply defaults (registry, tag)

	// Final validation after normalization
	if !IsValidImageReference(&ref) {
		// Include the normalized state in the error message for better debugging
		return nil, fmt.Errorf("parsing image reference '%s': invalid structure after parsing and normalization: %+v", imageStr, ref)
	}
	debug.Printf("Successfully parsed reference: %+v", ref)
	ref.Detected = true // Mark as detected by the parser itself
	return &ref, nil
}

// parseImageReferenceCustom provides a fallback parsing mechanism for image strings
// that might not strictly conform to the distribution reference format.
// This is a simplified version restored to fix linter errors; original might differ.
func parseImageReferenceCustom(imageStr string) (Reference, error) {
	debug.FunctionEnter("parseImageReferenceCustom")
	defer debug.FunctionExit("parseImageReferenceCustom")
	debug.Printf("Custom parsing image string: '%s'", imageStr)

	ref := Reference{Original: imageStr} // Initialize with original string
	namePart := imageStr
	var err error

	// Basic split logic (example, might need adjustment based on original code)
	if digestIdx := strings.LastIndexByte(imageStr, '@'); digestIdx != -1 {
		namePart = imageStr[:digestIdx]
		ref.Digest = imageStr[digestIdx+1:]
		if !isValidDigest(ref.Digest) {
			debug.Printf("Custom Parse Fail: Invalid Digest: %s", ref.Digest)
			return ref, fmt.Errorf("custom parse: invalid digest format in %s", imageStr)
		}
		debug.Printf("Custom Parse: Found digest '%s', name part '%s'", ref.Digest, namePart)
	} else if tagIdx := strings.LastIndexByte(imageStr, ':'); tagIdx != -1 {
		// Check if this colon is part of the registry (e.g., localhost:5000/repo)
		slashIdx := strings.IndexByte(imageStr, '/')
		if slashIdx == -1 || tagIdx > slashIdx { // Colon is for tag
			namePart = imageStr[:tagIdx]
			ref.Tag = imageStr[tagIdx+1:]
			if !isValidTag(ref.Tag) {
				debug.Printf("Custom Parse Fail: Invalid Tag: %s", ref.Tag)
				return ref, fmt.Errorf("custom parse: invalid tag format in %s", imageStr)
			}
			debug.Printf("Custom Parse: Found tag '%s', name part '%s'", ref.Tag, namePart)
		} else {
			// Colon is part of registry, no tag/digest detected here
			namePart = imageStr
			debug.Printf("Custom Parse: Colon assumed part of registry, name part '%s'", namePart)
		}
	} else {
		// No tag or digest separator found
		namePart = imageStr
		debug.Printf("Custom Parse: No tag or digest separator found, name part '%s'", namePart)
	}

	// Parse registry/repo from the remaining name part
	ref.Registry, ref.Repository, err = parseRegistryRepo(namePart, imageStr)
	if err != nil {
		debug.Printf("Custom Parse Fail: Error parsing registry/repo from '%s': %v", namePart, err)
		return ref, fmt.Errorf("custom parse: failed to parse registry/repo from '%s': %w", namePart, err)
	}

	debug.Printf("Custom Parse Result: %+v", ref)
	// Note: Normalization and full validation happens *after* this function returns
	return ref, nil
}

// parseRegistryRepo extracts the registry and repository from the name part of an image string.
// It returns the registry (or empty string if defaulted), repository, and an error if parsing fails.
func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) {
	if namePart == "" {
		// This case should ideally be caught earlier, but handle defensively.
		return "", "", fmt.Errorf("parsing image reference '%s': empty name part provided to parseRegistryRepo", imgStr)
	}

	slashIdx := strings.IndexByte(namePart, '/')
	if slashIdx == -1 {
		// Assumed to be repository only (registry defaulted later in normalization)
		if !isValidRepositoryName(namePart) {
			debug.Printf("Invalid repository name: '%s'", namePart)
			return "", "", fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
		}
		return "", namePart, nil
	} else {
		potentialRegistry := namePart[:slashIdx]
		potentialRepo := namePart[slashIdx+1:]

		// Treat as registry only if it contains '.', ':', or is 'localhost'
		if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
			if !isValidRegistryName(potentialRegistry) {
				debug.Printf("Invalid registry name inferred: '%s'", potentialRegistry)
				return "", "", fmt.Errorf("parsing image reference '%s': invalid registry name detected: %s", imgStr, potentialRegistry)
			}
			if potentialRepo == "" {
				// Allow registry without repo (e.g., docker.io/ -> docker.io/library)
				debug.Printf("Registry found ('%s') but repository part is empty. Will be normalized.", potentialRegistry)
			} else if !isValidRepositoryName(potentialRepo) {
				debug.Printf("Invalid repository name after registry split: '%s'", potentialRepo)
				return "", "", fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
			}
			return potentialRegistry, potentialRepo, nil
		} else {
			// No '.', ':' means the whole thing is the repository (e.g., myrepo/myapp)
			if !isValidRepositoryName(namePart) {
				debug.Printf("Invalid repository name (treated as whole): '%s'", namePart)
				return "", "", fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
			}
			return "", namePart, nil
		}
	}
}

// IsValidImageReference performs basic validation on a parsed ImageReference
// Note: This checks structure, not necessarily registry reachability etc.
func IsValidImageReference(ref *Reference) bool {
	debug.FunctionEnter("IsValidImageReference")
	defer debug.FunctionExit("IsValidImageReference")
	debug.DumpValue("Validating Reference", ref)

	// Validate Registry (if present)
	if ref.Registry != "" && !isValidRegistryName(ref.Registry) {
		debug.Printf("Validation Fail: Invalid Registry Name: %s", ref.Registry)
		return false
	}

	// Validate Repository (must be present and valid using the stricter internal multi-check function)
	if ref.Repository == "" || !isValidRepositoryName(ref.Repository) {
		debug.Printf("Validation Fail: Invalid Repository Name: %s", ref.Repository)
		return false
	}

	// Must have either a Tag or a Digest, but not both.
	hasTag := ref.Tag != ""
	hasDigest := ref.Digest != ""

	if hasTag && hasDigest {
		debug.Println("Validation Fail: Cannot have both Tag and Digest")
		return false // Cannot have both tag and digest
	}
	if !hasTag && !hasDigest {
		// This case should ideally be handled by normalization setting a default tag,
		// but we double-check here. An image ref must have one or the other.
		debug.Println("Validation Fail: Must have either Tag or Digest")
		return false
	}

	// Validate Tag if present
	if hasTag && !isValidTag(ref.Tag) {
		debug.Printf("Validation Fail: Invalid Tag: %s", ref.Tag)
		return false
	}

	// Validate Digest if present
	if hasDigest && !isValidDigest(ref.Digest) {
		debug.Printf("Validation Fail: Invalid Digest: %s", ref.Digest)
		return false
	}

	debug.Println("Validation Success")
	return true
}
