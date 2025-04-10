package image

import (
	"fmt"
	"net"     // Need this for port stripping
	"strings" // Need this for normalization checks

	"github.com/distribution/reference"
	"github.com/lalbers/irr/pkg/debug"
)

// Constants
const (
	// LatestTag is the default tag used when no tag is specified
	LatestTag = "latest"
)

// Errors
// var ( // REMOVED - Defined in errors.go
// 	ErrEmptyImageReference   = errors.New("image reference string cannot be empty") // REMOVED
// 	ErrTagAndDigestPresent = errors.New("image reference cannot contain both a tag and a digest") // REMOVED
// ) // REMOVED

// ParseImageReference attempts to parse a raw image string into a structured Reference.
// It uses the distribution/reference library for the core parsing logic.
//
// NOTE: This function currently has known limitations (See TODO.md Phase 3):
//   - It does not perform necessary pre-normalization (e.g., adding docker.io/library/, latest tag).
//   - It does not perform post-processing (e.g., stripping :port from registry/domain).
//   - These omissions cause failures in tests expecting normalized references or specific error types.
//   - The underlying distribution/reference.ParseNamed function has stricter requirements than
//     simple string splitting might imply (e.g., canonical repository names).
func ParseImageReference(imageStr string) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s'", imageStr)

	if imageStr == "" {
		debug.Println("Error: Input string is empty.")
		return nil, ErrEmptyImageReference
	}

	// --- Parsing using distribution/reference ---
	// Use ParseNormalizedNamed which handles adding defaults like docker.io/library/ and 'latest' tag implicitly.
	parsed, err := reference.ParseNormalizedNamed(imageStr)
	if err != nil {
		debug.Printf("ParseNormalizedNamed failed for '%s': %v", imageStr, err)
		// Check for specific known error types if needed, otherwise return a wrapped generic error.
		// The library might return ErrReferenceInvalidFormat or other specific errors.
		// Wrapping provides context while allowing checks via errors.Is or errors.As if necessary downstream.
		// Using fmt.Errorf with %v preserves the original error message details.
		return nil, fmt.Errorf("failed to parse image reference '%s' using distribution/reference: %w", imageStr, err)
	}
	debug.Printf("Successfully parsed '%s' with ParseNormalizedNamed: %s", imageStr, parsed.String())

	// --- Build Reference struct ---
	// Start with the original string and detected status
	ref := Reference{
		Original: imageStr,
		Detected: true,
	}

	// Extract components using library functions AFTER successful parsing
	ref.Registry = reference.Domain(parsed)
	ref.Repository = reference.Path(parsed)
	debug.Printf("Extracted Domain: %s, Path: %s", ref.Registry, ref.Repository)

	// --- Post-processing: Strip port from registry (Domain) ---
	// The Domain() function should return the registry *without* the port.
	// However, let's double-check and handle potential edge cases or library behavior changes.
	if strings.Contains(ref.Registry, ":") {
		registryHost, _, errPort := net.SplitHostPort(ref.Registry)
		if errPort == nil {
			debug.Printf("Registry '%s' contains a port. Stripping to '%s'.", ref.Registry, registryHost)
			ref.Registry = registryHost
		} else {
			// This case should ideally not happen if Domain() works as expected. Log a warning.
			debug.Printf("Warning: Could not split host/port for registry '%s' after successful parse: %v. Using original domain value.", ref.Registry, errPort)
		}
	}

	// --- Extract Tag and Digest ---
	var hasTag, hasDigest bool
	if tagged, ok := parsed.(reference.Tagged); ok {
		ref.Tag = tagged.Tag()
		hasTag = true
		debug.Printf("Extracted Tag: %s", ref.Tag)
	}
	if digested, ok := parsed.(reference.Digested); ok {
		// Important: Digest() returns a digest.Digest object. Use String() for the string representation.
		ref.Digest = digested.Digest().String()
		hasDigest = true
		debug.Printf("Extracted Digest: %s", ref.Digest)
	}

	// --- Validation: Ensure Tag and Digest are not both present ---
	// ParseNormalizedNamed might technically allow parsing strings with both,
	// but semantically, a reference usually shouldn't have both specified this way.
	if hasTag && hasDigest {
		debug.Printf("Validation Fail: Both Tag ('%s') and Digest ('%s') are present after parsing '%s'", ref.Tag, ref.Digest, imageStr)
		// Return the specific exported error for this condition for clear identification.
		// Pass the conflicting parts for a more informative error message.
		return nil, fmt.Errorf("%w: reference '%s' contained both tag (%s) and digest (%s)",
			ErrTagAndDigestPresent, imageStr, ref.Tag, ref.Digest)
	}

	// --- Post-processing: Add 'latest' tag if missing ---
	// reference.ParseNormalizedNamed *should* add 'latest' if no tag/digest is present.
	// However, let's explicitly ensure ref.Tag is set if neither was found.
	if !hasTag && !hasDigest {
		// This might occur if ParseNormalizedNamed logic changes or for unusual inputs.
		debug.Printf("Neither tag nor digest was extracted for '%s' (parsed: %s). Ensuring 'latest' tag.", imageStr, parsed.String())
		ref.Tag = "latest"
	} else if hasTag && ref.Tag == "" {
		// Handle case where it's Tagged, but the tag is empty (shouldn't happen with valid refs)
		debug.Printf("Warning: Parsed reference '%s' was Tagged but has an empty tag. Setting to 'latest'.", parsed.String())
		ref.Tag = "latest"
	}

	// Repository name validation is handled by reference.ParseNamed.

	debug.Printf("Successfully parsed and processed reference: %+v", ref)
	return &ref, nil
}

// // parseImageReferenceCustom is deprecated. // REMOVED UNUSED
// func parseImageReferenceCustom(imageStr string) (Reference, error) { // REMOVED UNUSED
// 	return Reference{}, errors.New("parseImageReferenceCustom is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED
// // parseRegistryRepo is deprecated. // REMOVED UNUSED
// func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) { // REMOVED UNUSED
// 	return "", "", errors.New("parseRegistryRepo is deprecated and should not be called") // REMOVED UNUSED
// } // REMOVED UNUSED
