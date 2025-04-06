package image

import "errors"

// Sentinel errors related to image parsing and handling.
var (
	// Errors related to map structure parsing
	ErrInvalidImageMapRepo         = errors.New("invalid image map: repository is not a string")
	ErrInvalidImageMapRegistryType = errors.New("invalid image map: registry is not a string")
	ErrInvalidImageMapTagType      = errors.New("invalid image map: tag is not a string")
	ErrRepoNotFound                = errors.New("repository not found or not a string")

	// Errors related to string parsing
	ErrEmptyImageString       = errors.New("cannot parse empty image string")
	ErrInvalidDigestFormat    = errors.New("invalid digest format")
	ErrInvalidTagFormat       = errors.New("invalid tag format")
	ErrInvalidRepoName        = errors.New("invalid repository name")
	ErrInvalidImageRefFormat  = errors.New("invalid image reference format")
	ErrInvalidRegistryName    = errors.New("invalid registry name")
	ErrInvalidImageString     = errors.New("invalid image string format")
	ErrTemplateVariableInRepo = errors.New("template variable detected in repository string")
	ErrInvalidTypeAssertion   = errors.New("invalid type assertion during image detection")

	// Common validation errors
	ErrUnsupportedImageType = errors.New("unsupported image reference type")
	ErrMissingTagOrDigest   = errors.New("missing tag or digest")
	ErrTagAndDigestPresent  = errors.New("both tag and digest present")
)

// Sentinel errors related to path manipulation (originally in path_utils.go).
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
