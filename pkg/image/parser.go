package image

import (
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
)

const (
	// Patterns for detecting image references
	tagPattern = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?` +
		`(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9]):` +
		`(?P<tag>[a-zA-Z0-9][-a-zA-Z0-9._]+)$`
	digestPattern = `^(?:(?P<registry>[a-zA-Z0-9][-a-zA-Z0-9.]*[a-zA-Z0-9](:[0-9]+)?)/)?` +
		`(?P<repository>[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9])@` +
		`(?P<digest>sha256:[a-fA-F0-9]{64})$`

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

	// Check for invalid tag + digest combination first
	digestIndexCheck := strings.Index(imgStr, "@sha256:")
	if digestIndexCheck > 0 {
		tagIndexCheck := strings.LastIndex(imgStr[:digestIndexCheck], ":")
		slashIndexCheck := strings.Index(imgStr, "/")
		if tagIndexCheck > 0 && (slashIndexCheck == -1 || tagIndexCheck > slashIndexCheck) {
			debug.Printf("Error: Found both tag separator ':' and digest separator '@' in '%s'", imgStr)
			return nil, ErrTagAndDigestPresent
		}
	}

	var ref Reference
	ref.Original = imgStr // Set the Original field to input string

	// --- Prioritize Digest Parsing ---
	if strings.Contains(imgStr, "@") {
		parts := strings.SplitN(imgStr, "@", maxSplitTwo)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			debug.Printf("Invalid format for digest reference: '%s'", imgStr)
			return nil, ErrInvalidImageRefFormat
		}
		namePart := parts[0]
		digestPart := parts[1]

		// Validate digest part strictly
		if !isValidDigest(digestPart) {
			debug.Printf("Invalid digest format in '%s'", digestPart)
			return nil, ErrInvalidDigestFormat
		}
		ref.Digest = digestPart // Store the valid digest

		// Parse the name part (registry/repository)
		slashIndex := strings.Index(namePart, "/")
		if slashIndex == -1 {
			// Assumed to be repository only (e.g., "myimage@sha256:...")
			if !isValidRepositoryName(namePart) {
				debug.Printf("Invalid repository name in digest reference: '%s'", namePart)
				return nil, ErrInvalidRepoName
			}
			ref.Repository = namePart
			// Registry will be defaulted by NormalizeImageReference
		} else {
			// Potential registry/repository split
			potentialRegistry := namePart[:slashIndex]
			potentialRepo := namePart[slashIndex+1:]

			// Basic check: if first part contains '.' or ':', assume it's a registry
			if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
				if !isValidRegistryName(potentialRegistry) { // Optional stricter check?
					debug.Printf("Invalid registry name inferred: '%s'", potentialRegistry)
					// Treat as error or proceed assuming it's part of the repo? Let's be strict for now.
					// return nil, ErrInvalidRegistryName // Need this error defined
					return nil, fmt.Errorf("invalid registry name detected: %s", potentialRegistry)
				}
				if !isValidRepositoryName(potentialRepo) {
					debug.Printf("Invalid repository name after registry split: '%s'", potentialRepo)
					return nil, ErrInvalidRepoName
				}
				ref.Registry = potentialRegistry
				ref.Repository = potentialRepo
			} else {
				// Assume the whole namepart is the repository
				if !isValidRepositoryName(namePart) {
					debug.Printf("Invalid repository name (treated as whole): '%s'", namePart)
					return nil, ErrInvalidRepoName
				}
				ref.Repository = namePart
				// Registry will be defaulted by NormalizeImageReference
			}
		}

		// Make sure repository is not empty after parsing name part
		if ref.Repository == "" {
			debug.Println("Error: Missing repository in digest reference after parsing name.")
			return nil, ErrInvalidRepoName
		}

		NormalizeImageReference(&ref) // Normalize AFTER validation and parsing
		debug.Printf("Parsed digest ref: Registry='%s', Repo='%s', Digest='%s'", ref.Registry, ref.Repository, ref.Digest)
		return &ref, nil
	}

	// --- Tag-Based Parsing ---
	tagIndex := strings.LastIndex(imgStr, ":")
	slashIndex := strings.Index(imgStr, "/") // First slash

	if tagIndex == -1 {
		// No colon: Assume it's just a repository name (tag defaults to 'latest')
		if !isValidRepositoryName(imgStr) {
			debug.Printf("Invalid repository name (no tag/digest): '%s'", imgStr)
			return nil, ErrInvalidRepoName
		}
		ref.Repository = imgStr
		// ref.Tag = "latest" // Let Normalize handle default tag
	} else {
		// Colon found
		// Check if colon is part of a port number in the registry part
		if slashIndex != -1 && tagIndex < slashIndex {
			// Colon appears before the first slash, likely part of a port number
			// e.g., "myregistry.com:5000/myrepo" (no tag specified)
			if !isValidRepositoryName(imgStr) { // Validate the whole string as repo part
				debug.Printf("Invalid repository name (colon before slash): '%s'", imgStr)
				return nil, ErrInvalidRepoName
			}
			ref.Repository = imgStr
			// ref.Tag = "latest" // Let Normalize handle default tag
		} else {
			// Colon appears after the first slash or no slash exists, assume it separates repo/tag
			namePart := imgStr[:tagIndex]
			tagPart := imgStr[tagIndex+1:]

			if namePart == "" || tagPart == "" {
				debug.Printf("Invalid format near tag separator: '%s'", imgStr)
				return nil, ErrInvalidImageRefFormat // Missing repo or tag
			}
			if !isValidTag(tagPart) {
				debug.Printf("Invalid tag format: '%s'", tagPart)
				return nil, ErrInvalidTagFormat
			}
			ref.Tag = tagPart

			// Parse the name part (registry/repository)
			slashIndexName := strings.Index(namePart, "/")
			if slashIndexName == -1 {
				// Assumed to be repository only (e.g., "myimage:tag")
				if !isValidRepositoryName(namePart) {
					debug.Printf("Invalid repository name (tag present): '%s'", namePart)
					return nil, ErrInvalidRepoName
				}
				ref.Repository = namePart
			} else {
				// Potential registry/repository split
				potentialRegistry := namePart[:slashIndexName]
				potentialRepo := namePart[slashIndexName+1:]

				if strings.ContainsAny(potentialRegistry, ".:") || potentialRegistry == "localhost" {
					if !isValidRegistryName(potentialRegistry) { // Optional stricter check?
						debug.Printf("Invalid registry name inferred (tag): '%s'", potentialRegistry)
						// return nil, ErrInvalidRegistryName
						return nil, fmt.Errorf("invalid registry name detected: %s", potentialRegistry)

					}
					if !isValidRepositoryName(potentialRepo) {
						debug.Printf("Invalid repository name after registry split (tag): '%s'", potentialRepo)
						return nil, ErrInvalidRepoName
					}
					ref.Registry = potentialRegistry
					ref.Repository = potentialRepo
				} else {
					// Assume the whole namepart is the repository
					if !isValidRepositoryName(namePart) {
						debug.Printf("Invalid repository name (treated as whole, tag): '%s'", namePart)
						return nil, ErrInvalidRepoName
					}
					ref.Repository = namePart
				}
			}
			// Make sure repository is not empty after parsing name part
			if ref.Repository == "" {
				debug.Println("Error: Missing repository in tag reference after parsing name.")
				return nil, ErrInvalidRepoName
			}
		}
	}

	// If tag is missing, default to latest. This might be overridden by normalization later if digest is found.
	if ref.Tag == "" && ref.Digest == "" {
		// ref.Tag = "latest" // Let Normalize handle default tag
	}

	NormalizeImageReference(&ref) // Normalize AFTER validation and parsing
	debug.Printf("Successfully parsed tag-based reference: %+v", ref)
	ref.Detected = true
	return &ref, nil
}

// IsValidImageReference performs basic validation on a parsed ImageReference
// Note: This checks structure, not necessarily registry reachability etc.
func IsValidImageReference(ref *Reference) bool {
	if ref == nil {
		return false
	}
	if ref.Repository == "" {
		return false
	}
	if ref.Tag == "" && ref.Digest == "" {
		// Allow if it might be a template tag
		// How to reliably detect this without full template engine?
		// For now, allow empty tag/digest if potentially templated.
		// A stricter validation might happen later.
		return true // Looser check, assume templates are possible
	}
	if ref.Tag != "" && ref.Digest != "" {
		return false // Cannot have both
	}
	return true
}

// looksLikeImageReference is a helper function to quickly check if a string resembles an image reference format.
// This is less strict than full parsing and used for heuristics in strict mode.
func looksLikeImageReference(s string) bool {
	// Check for required separator (: for tag or @ for digest)
	hasSeparator := strings.Contains(s, ":") || strings.Contains(s, "@")
	if !hasSeparator {
		return false // Must have one of the separators
	}

	// Avoid matching obvious file paths or URLs
	hasSlash := strings.Contains(s, "/")
	isFilePath := hasSlash && (strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../"))
	isURL := strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
	if isFilePath || isURL {
		return false
	}

	// Further check: if no slash, ensure the part before the separator is simple
	if !hasSlash {
		sepIndex := strings.LastIndex(s, ":")
		if sepIndex == -1 {
			sepIndex = strings.LastIndex(s, "@")
		}
		if sepIndex > 0 {
			potentialRepo := s[:sepIndex]
			// Basic check for plausible repo name (avoid things like 'http:')
			if strings.ContainsAny(potentialRepo, "./") { // If it contains path separators without a registry-like part, likely not an image
				return false
			}
		}
	}

	// If it has a separator and doesn't look like a file path/URL, consider it plausible.
	return true
}
