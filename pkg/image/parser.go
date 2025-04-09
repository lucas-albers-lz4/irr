package image

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

// Constants for regex patterns
// Remove unused tagPattern and digestPattern

// Full pattern combining registry, repository, and tag/digest parts
const referencePattern = `^` +
	`(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?` + // Registry (optional)
	`(?P<repository>[a-zA-Z0-9_./-]+)` + // Repository (required)
	`(?:(?P<separator>[:@])` + // Separator (: or @)
	`(?P<tagordigest>[a-zA-Z0-9_.-]+(?:[.+_-][a-zA-Z0-9_.-]+)*))?` + // Tag or Digest (optional)
	`$`

// Compiled regex for the full reference
var compiledReferenceRegex = regexp.MustCompile(referencePattern)

const (
	// maxSplitTwo is the limit for splitting into at most two parts
	maxSplitTwo = 2
)

// ParseImageReference parses a standard Docker image reference string (e.g., registry/repo:tag or repo@digest)
// It returns an ImageReference or an error if parsing fails.
func ParseImageReference(imgStr string) (*Reference, error) {
	debug.FunctionEnter("ParseImageReference")
	defer debug.FunctionExit("ParseImageReference")
	debug.Printf("Parsing image string: '%s'", imgStr)

	if imgStr == "" {
		debug.Println("Error: Input string is empty.")
		return nil, ErrEmptyImageReference // Use canonical error
	}

	var ref Reference
	ref.Original = imgStr // Set the Original field to input string

	// --- Determine structure and check for immediate invalid patterns ---
	lastAt := strings.LastIndex(imgStr, "@")
	lastColon := strings.LastIndex(imgStr, ":")
	firstSlash := strings.Index(imgStr, "/")

	hasPotentialDigestSeparator := lastAt > 0 // Check if '@' exists
	hasPotentialTagSeparator := lastColon > 0 // Check if ':' exists

	// Initial check for invalid tag:digest pattern like "image:tag@sha256:..."
	// This specific pattern is always invalid.
	if hasPotentialDigestSeparator && hasPotentialTagSeparator && lastColon < lastAt {
		// Ensure the colon is after the first slash if a slash exists (to avoid matching ports)
		if firstSlash == -1 || lastColon > firstSlash {
			// And ensure the part after @ looks like a digest
			if strings.HasPrefix(imgStr[lastAt+1:], "sha256:") {
				debug.Printf("Error: Found tag separator ':' before digest separator '@sha256:' in '%s'", imgStr)
				return nil, ErrTagAndDigestPresent
			}
		}
	}

	isLikelyDigest := false
	isLikelyTag := false

	if hasPotentialDigestSeparator {
		if strings.HasPrefix(imgStr[lastAt+1:], "sha256:") {
			isLikelyDigest = true // It looks like a valid digest structure
		} else {
			// Contains '@' but not starting "sha256:".
			// This is ambiguous. If a non-port colon also exists, it's likely an invalid repo name.
			// Otherwise, it's an invalid digest format.
			if hasPotentialTagSeparator && (firstSlash == -1 || lastColon > firstSlash) {
				debug.Printf("Error: Ambiguous format with both '@' and ':' separators, and '@' does not start a sha256 digest: '%s'", imgStr)
				return nil, fmt.Errorf("parsing image reference '%s': %w (ambiguous separators found)", imgStr, ErrInvalidRepoName)
			} else {
				// Contains '@' but not sha256:, and NO non-port tag separator. Treat as invalid digest format.
				debug.Printf("Invalid digest format (does not start with sha256:): '%s'", imgStr[lastAt+1:])
				return nil, ErrInvalidDigestFormat
			}
		}
	} else if hasPotentialTagSeparator && (firstSlash == -1 || lastColon > firstSlash) {
		// Only consider it a tag if the colon is not part of a port
		isLikelyTag = true
	}

	// --- Parse Based on Determined Structure ---

	if isLikelyDigest {
		// --- Digest Parsing ---
		parts := strings.SplitN(imgStr, "@", maxSplitTwo)
		namePart := parts[0]
		digestPart := parts[1] // Contains the full digest string, e.g., "sha256:..."

		// Validate digest part strictly
		if !isValidDigest(digestPart) {
			debug.Printf("Invalid digest format in '%s'", digestPart)
			return nil, ErrInvalidDigestFormat // Should be caught earlier, but re-check
		}
		ref.Digest = digestPart

		// --> ADDED CHECK <--
		// Check if the name part *also* contains a tag separator (invalid)
		lastColonName := strings.LastIndex(namePart, ":")
		firstSlashName := strings.Index(namePart, "/")
		if lastColonName > 0 && (firstSlashName == -1 || lastColonName > firstSlashName) {
			debug.Printf("Error: Found tag separator ':' in name part before digest: '%s'", namePart)
			return nil, ErrTagAndDigestPresent
		}
		// --> END ADDED CHECK <--

		// Parse the name part (registry/repository)
		var err error
		ref.Registry, ref.Repository, err = parseRegistryRepo(namePart, imgStr)
		if err != nil {
			return nil, err // Error already contains context
		}

		// Check for tag presence before digest - should be invalid
		if strings.Contains(namePart, ":") {
			// Scan for a colon that is NOT part of a port number in the registry part
			colonIdx := strings.LastIndexByte(namePart, ':')
			if colonIdx > strings.LastIndexByte(namePart, '/') { // Colon is likely a tag separator
				// Check if it looks like a port number
				if _, errConv := strconv.Atoi(namePart[colonIdx+1:]); errConv != nil {
					// It's not a port number, likely a tag - error!
					debug.Printf("Found tag ':' before digest '@' in '%s'", imgStr)
					return nil, fmt.Errorf("%w: cannot have tag and digest: %s", ErrTagAndDigestPresent, imgStr)
				}
			}
		}

	} else if isLikelyTag {
		// --- Tag Parsing ---
		if firstSlash != -1 && lastColon < firstSlash {
			// Colon is part of port: registry:port/repo
			potentialRegistry := imgStr[:firstSlash]
			potentialRepo := imgStr[firstSlash+1:]
			if potentialRegistry == "" || potentialRepo == "" {
				debug.Printf("Error: Missing registry or repository in image reference: '%s'", imgStr)
				return nil, fmt.Errorf("parsing image reference '%s': %w (missing registry or repository)", imgStr, ErrInvalidRepoName)
			}
			if !isValidRegistryName(potentialRegistry) {
				debug.Printf("Invalid registry name inferred: '%s'", potentialRegistry)
				return nil, fmt.Errorf("parsing image reference '%s': invalid registry name detected: %s", imgStr, potentialRegistry)
			}
			if !isValidRepositoryName(potentialRepo) {
				debug.Printf("Invalid repository name after registry split: '%s'", potentialRepo)
				return nil, fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
			}
			ref.Registry = potentialRegistry
			ref.Repository = potentialRepo
			// Tag defaulted later
		} else {
			// Contains ':' but no '@', assume tag format
			lastColon := strings.LastIndexByte(imgStr, ':')
			namePart := imgStr[:lastColon]
			tagPart := imgStr[lastColon+1:]

			if !isValidTag(tagPart) {
				debug.Printf("Invalid tag format: '%s'", tagPart)
				return nil, ErrInvalidTagFormat
			}
			ref.Tag = tagPart

			// Parse the name part (registry/repository)
			var err error
			ref.Registry, ref.Repository, err = parseRegistryRepo(namePart, imgStr)
			if err != nil {
				return nil, err // Error already contains context
			}

			// Now, double-check there's no '@' digest hiding in the name part
			if strings.Contains(namePart, "@") {
				return nil, fmt.Errorf("parsing image reference '%s': found '@' in name part when tag ':' is present", imgStr)
			}
		}

	} else {
		// --- Repository/Registry Only Parsing ---
		if firstSlash != -1 {
			// Slash found: Split registry/repo
			potentialRegistry := imgStr[:firstSlash]
			potentialRepo := imgStr[firstSlash+1:]
			if potentialRegistry == "" || potentialRepo == "" {
				debug.Printf("Error: Missing registry or repository in image reference: '%s'", imgStr)
				return nil, fmt.Errorf("parsing image reference '%s': %w (missing registry or repository)", imgStr, ErrInvalidRepoName)
			}
			if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
				if !isValidRegistryName(potentialRegistry) {
					debug.Printf("Invalid registry name inferred: '%s'", potentialRegistry)
					return nil, fmt.Errorf("parsing image reference '%s': invalid registry name detected: %s", imgStr, potentialRegistry)
				}
				if !isValidRepositoryName(potentialRepo) {
					debug.Printf("Invalid repository name after registry split: '%s'", potentialRepo)
					return nil, fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
				}
				ref.Registry = potentialRegistry
				ref.Repository = potentialRepo
			} else {
				if !isValidRepositoryName(imgStr) {
					debug.Printf("Invalid repository name (treated as whole): '%s'", imgStr)
					return nil, fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
				}
				ref.Repository = imgStr // Registry defaulted later
			}
		} else {
			// No slash: Just repository name
			if !isValidRepositoryName(imgStr) {
				debug.Printf("Invalid repository name (treated as whole): '%s'", imgStr)
				return nil, fmt.Errorf("parsing image reference '%s': %w", imgStr, ErrInvalidRepoName)
			}
			ref.Repository = imgStr // Registry defaulted later
		}
		// Tag defaulted later
	}

	NormalizeImageReference(&ref) // Normalize AFTER parsing
	// Final validation after normalization
	if !IsValidImageReference(&ref) {
		// Include the normalized state in the error message for better debugging
		return nil, fmt.Errorf("parsing image reference '%s': invalid structure after parsing and normalization: %+v", imgStr, ref)
	}
	debug.Printf("Successfully parsed reference: %+v", ref)
	ref.Detected = true // Mark as detected by the parser itself
	return &ref, nil
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

	// Validate Repository (must be present and valid)
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
