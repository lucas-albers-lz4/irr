package image

import "errors"

// Package image defines errors related to image reference parsing and detection.
//
// This file contains all error definitions for the image package.
// DO NOT define package errors in other files.
// When adding new errors, follow these guidelines:
// 1. Group related errors together
// 2. Use consistent naming: Err[Component][Description]
// 3. Provide descriptive error messages
// 4. Add a comment explaining when the error occurs

// Sentinel errors related to image parsing and handling.
var (
	// --- Map Structure Parsing Errors ---

	// ErrInvalidImageMapRepo occurs when the 'repository' field in an image map is not a string.
	ErrInvalidImageMapRepo = errors.New("image map has invalid repository type (must be string)")
	// ErrInvalidImageMapRegistryType occurs when the 'registry' field in an image map is not a string.
	ErrInvalidImageMapRegistryType = errors.New("image map has invalid registry type (must be string)")
	// ErrInvalidImageMapTagType occurs when the 'tag' field in an image map is not a string.
	ErrInvalidImageMapTagType = errors.New("image map has invalid tag type (must be string)")
	// ErrInvalidImageMapDigestType occurs when the 'digest' field in an image map is not a string.
	ErrInvalidImageMapDigestType = errors.New("image map has invalid digest type (must be string)")
	// ErrRepoNotFound occurs when the 'repository' key is missing or not a string in an image map.
	ErrRepoNotFound = errors.New("repository key not found in image map") // Should logically be caught earlier
	// ErrMissingRepoInImageMap occurs when the 'repository' key is missing in an image map.
	ErrMissingRepoInImageMap = errors.New("image map is missing required 'repository' key")

	// --- String Parsing Errors ---

	// ErrEmptyImageString occurs when trying to parse an empty string as an image reference.
	ErrEmptyImageString = errors.New("cannot parse empty image string")
	// ErrEmptyImageReference occurs when an image reference string provided is empty.
	ErrEmptyImageReference = errors.New("image reference string cannot be empty")
	// ErrInvalidDigestFormat occurs when a digest part does not match the expected 'sha256:...' format.
	ErrInvalidDigestFormat = errors.New("invalid digest format")
	// ErrInvalidTagFormat occurs when a tag part contains invalid characters.
	ErrInvalidTagFormat = errors.New("invalid tag format")
	// ErrInvalidRepoName occurs when a repository name part contains invalid characters or format.
	ErrInvalidRepoName = errors.New("invalid repository name")
	// ErrInvalidImageRefFormat occurs for general format violations in an image reference string.
	ErrInvalidImageRefFormat = errors.New("invalid image reference format")
	// ErrInvalidRegistryName occurs when a registry name part contains invalid characters or format.
	ErrInvalidRegistryName = errors.New("invalid registry name")
	// ErrInvalidImageString occurs for general format violations in an image string value found in Helm values.
	ErrInvalidImageString = errors.New("invalid image string format")
	// ErrTemplateVariableInRepo occurs when a Go template construct is detected within the repository part of an image string.
	ErrTemplateVariableInRepo = errors.New("template variable detected in repository string")
	// ErrInvalidTypeAssertion occurs during recursive value traversal when an expected type assertion fails.
	ErrInvalidTypeAssertion = errors.New("invalid type assertion during image detection") // Consider making more specific if possible

	// --- Common Validation Errors ---

	// ErrUnsupportedImageType occurs when the detected structure (map/string) is not supported or recognized.
	ErrUnsupportedImageType = errors.New("unsupported image reference type")
	// ErrMissingTagOrDigest occurs when an image map or string lacks both a tag and a digest.
	ErrMissingTagOrDigest = errors.New("missing tag or digest")
	// ErrTagAndDigestPresent occurs when an image map or string specifies both a tag and a digest.
	ErrTagAndDigestPresent = errors.New("image cannot have both tag and digest specified")
	// ErrInvalidImageReference is a general error for invalid references after parsing.
	ErrInvalidImageReference = errors.New("invalid image reference") // Keep or reconcile? Keeping for now.

	// String parsing errors
	ErrAmbiguousStringPath = errors.New("string found at path not typically used for images, but resembles an image reference")
)

// Sentinel errors related to path manipulation (originally in path_utils.go).
// Ensure these are used consistently by path utility functions.
var (
	ErrEmptyPath                = errors.New("path cannot be empty")
	ErrPathNotFound             = errors.New("path not found")
	ErrPathElementNotMap        = errors.New("intermediate path element is not a map")
	ErrPathElementNotSlice      = errors.New("intermediate path element is not a slice")
	ErrArrayIndexOutOfBounds    = errors.New("array index out of bounds")
	ErrInvalidArrayIndex        = errors.New("invalid array index")
	ErrCannotOverwriteStructure = errors.New("cannot overwrite existing map or slice with a value")
	ErrArrayIndexAsOnlyElement  = errors.New("cannot have array index as only path element")
)
