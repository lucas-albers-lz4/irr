package image

import (
	"errors"
	"fmt"

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

// Error definitions
var (
	ErrEmptyImageReference = errors.New("image reference cannot be empty")
	// ErrInvalidRepoName indicates that the parsed repository name is invalid.
	// DEPRECATED: Validation now relies on distribution/reference.
	// var ErrInvalidRepoName = errors.New("invalid repository name")
	ErrTagAndDigestPresent = errors.New("image reference cannot contain both a tag and a digest")
)

// ParseImageReference parses an image reference string into a structured Reference object.
// It uses the distribution/reference library for robust, spec-compliant parsing and normalization.
func ParseImageReference(imageStr string) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s'", imageStr)

	if imageStr == "" {
		debug.Println("Error: Input string is empty.")
		return nil, ErrEmptyImageReference // Use canonical error
	}

	// Use the distribution library for robust parsing.
	// ParseNamed normalizes the name (adds docker.io, library/, potentially latest tag implicitly)
	parsed, err := reference.ParseNamed(imageStr)
	if err != nil {
		debug.Printf("Distribution parse failed for '%s': %v.", imageStr, err)
		// Simply wrap the distribution error.
		return nil, fmt.Errorf("failed to parse image reference '%s': %w", imageStr, err)
	}

	// Distribution parsing succeeded, build Reference object from parsed data.
	ref := Reference{
		Registry:   reference.Domain(parsed),
		Repository: reference.Path(parsed),
		Original:   imageStr, // Always store the original input
		Detected:   true,     // Mark as detected by the parser
	}

	// Extract tag and digest using type assertions.
	var hasTag, hasDigest bool
	if tagged, ok := parsed.(reference.Tagged); ok {
		ref.Tag = tagged.Tag()
		hasTag = true
	}
	if digested, ok := parsed.(reference.Digested); ok {
		ref.Digest = digested.Digest().String()
		hasDigest = true
	}

	// Enforce our rule: cannot have both tag and digest.
	// This check is needed because ParseNamed might allow ambiguous refs that resolve to both.
	if hasTag && hasDigest {
		debug.Printf("Validation Fail: Both Tag ('%s') and Digest ('%s') are present after parsing '%s'", ref.Tag, ref.Digest, imageStr)
		return nil, fmt.Errorf("%w: reference '%s' resolved to both tag and digest", ErrTagAndDigestPresent, imageStr)
	}

	// Apply 'latest' tag if neither tag nor digest was found (Docker convention).
	// ParseNamed often normalizes repository-only names to include latest, but this ensures it.
	if !hasTag && !hasDigest {
		debug.Printf("Neither tag nor digest found for '%s', setting tag to 'latest'", imageStr)
		ref.Tag = "latest"
	}

	// Final sanity check on repository part (might be redundant)
	if !isValidRepositoryName(ref.Repository) {
		debug.Printf("Validation Fail: Invalid Repository Name after parsing: %s", ref.Repository)
		return nil, fmt.Errorf("%w: parsed repository '%s' is invalid", ErrInvalidRepoName, ref.Repository)
	}

	debug.Printf("Successfully parsed reference: %+v", ref)
	return &ref, nil
}

// parseImageReferenceCustom provides a fallback parsing mechanism for image strings
// that might not strictly conform to the distribution reference format.
// DEPRECATED: This function is no longer used as parsing relies solely on distribution/reference.
func parseImageReferenceCustom(imageStr string) (Reference, error) {
	return Reference{}, errors.New("parseImageReferenceCustom is deprecated and should not be called")
}

// parseRegistryRepo extracts the registry and repository from the name part of an image string.
// DEPRECATED: This function is no longer used as parsing relies solely on distribution/reference.
func parseRegistryRepo(namePart, imgStr string) (registry string, repository string, err error) {
	return "", "", errors.New("parseRegistryRepo is deprecated and should not be called")
}

// IsValidImageReference performs basic validation on a parsed ImageReference
// DEPRECATED: Validation is now primarily handled by distribution/reference parsing.
// Kept for potential internal use or future stricter checks if needed.
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
	// REPOSITORY VALIDATION REMOVED - Handled by ParseNamed
	// if ref.Repository == "" || !isValidRepositoryName(ref.Repository) {
	// 	debug.Printf("Validation Fail: Invalid Repository Name: %s", ref.Repository)
	// 	return false
	// }

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
