package image

import (
	"errors"
	"fmt"
	"net"     // Need this for port stripping
	"strings" // Need this for normalization checks

	"github.com/distribution/reference"
	"github.com/lalbers/irr/pkg/debug"
)

// Constants
const (
	maxSplitTwo = 2
)

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
	// Use ParseNormalizedNamed which handles adding defaults like docker.io/library/
	parsed, err := reference.ParseNormalizedNamed(imageStr)
	if err != nil {
		// Remove forced debug prints for error block
		debug.Printf("ParseNormalizedNamed failed for '%s': %v", imageStr, err)
		// Error message no longer needs to mention normalization explicitly
		return nil, fmt.Errorf("failed to parse image reference '%s': %w", imageStr, err)
	}
	// If we reach here, parsing was successful.

	// --- Build Reference struct ---
	ref := Reference{
		Registry:   reference.Domain(parsed),
		Repository: reference.Path(parsed),
		Original:   imageStr, // Always store the original input string
		Detected:   true,     // Mark as detected by the parser
	}

	// --- Post-processing: Strip port from registry ---
	if strings.Contains(ref.Registry, ":") {
		registryHost, _, errPort := net.SplitHostPort(ref.Registry)
		if errPort == nil {
			debug.Printf("Stripping port from registry: '%s' -> '%s'", ref.Registry, registryHost)
			ref.Registry = registryHost
		} else {
			debug.Printf("Warning: Could not split host/port for registry '%s' after successful parse: %v", ref.Registry, errPort)
			// Continue without stripping port if SplitHostPort fails unexpectedly
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
		ref.Digest = digested.Digest().String() // Use .String() for consistent format
		hasDigest = true
		debug.Printf("Extracted Digest: %s", ref.Digest)
	}

	// --- Validation ---
	// Enforce: cannot have both tag and digest. ParseNamed might allow ambiguous refs.
	if hasTag && hasDigest {
		debug.Printf("Validation Fail: Both Tag ('%s') and Digest ('%s') are present after parsing '%s'", ref.Tag, ref.Digest, imageStr)
		return nil, fmt.Errorf("%w: reference '%s' resolved to both tag and digest", ErrTagAndDigestPresent, imageStr)
	}

	// Explicitly add ':latest' tag if neither tag nor digest was found after parsing.
	// (Double-checking as ParseNamed's implicit latest might not always apply)
	if !hasTag && !hasDigest {
		debug.Printf("Neither tag nor digest found for parsed '%s', explicitly setting tag to 'latest'", parsed.String())
		ref.Tag = "latest"
	}

	// Repository name validation is handled by reference.ParseNamed.

	debug.Printf("Successfully parsed reference: %+v", ref)
	return &ref, nil
}

// parseImageReferenceCustom is deprecated.
func parseImageReferenceCustom(imageStr string) (Reference, error) {
	return Reference{}, errors.New("parseImageReferenceCustom is deprecated and should not be called")
}

// parseRegistryRepo is deprecated.
func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) {
	return "", "", errors.New("parseRegistryRepo is deprecated and should not be called")
}
