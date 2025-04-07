package image

import "fmt"

// Reference encapsulates the components of a container image reference.
type Reference struct {
	Original   string // The original string detected
	Registry   string // e.g., docker.io, quay.io, gcr.io
	Repository string
	Tag        string
	Digest     string
	Path       []string // Path in the values structure where this reference was found
	Detected   bool
}

// String returns the string representation of the image reference
func (r *Reference) String() string {
	if r.Registry != "" {
		if r.Digest != "" {
			return fmt.Sprintf("%s/%s@%s", r.Registry, r.Repository, r.Digest)
		}
		return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, r.Tag)
	}

	if r.Digest != "" {
		return fmt.Sprintf("%s@%s", r.Repository, r.Digest)
	}
	return fmt.Sprintf("%s:%s", r.Repository, r.Tag)
}

// LocationType defines how an image reference was structured in the original values.
type LocationType int

const (
	// TypeUnknown indicates an undetermined structure.
	TypeUnknown LocationType = iota
	// TypeMapRegistryRepositoryTag represents map{registry: "", repository: "", tag: ""}
	TypeMapRegistryRepositoryTag
	// TypeRepositoryTag represents map{repository: "", tag: ""} or map{image: ""} (if image contains repo+tag)
	TypeRepositoryTag
	// TypeString represents a simple string value like "repository:tag".
	TypeString
)

// DetectionContext holds configuration for image detection
type DetectionContext struct {
	SourceRegistries  []string
	ExcludeRegistries []string
	GlobalRegistry    string
	Strict            bool
	TemplateMode      bool
}

// DetectedImage represents an image found during detection
type DetectedImage struct {
	Reference *Reference
	Path      []string
	Pattern   string      // "map", "string", "global"
	Original  interface{} // Original value (for template preservation)
}

// UnsupportedImage represents an unsupported image found during detection
type UnsupportedImage struct {
	Location []string
	Type     UnsupportedType
	Error    error
}

// UnsupportedType defines the type of unsupported structure encountered.
type UnsupportedType int

const (
	// UnsupportedTypeUnknown indicates an unspecified or unknown unsupported type.
	UnsupportedTypeUnknown UnsupportedType = iota
	// UnsupportedTypeMap indicates an unsupported map structure.
	UnsupportedTypeMap
	// UnsupportedTypeString indicates an unsupported string format.
	UnsupportedTypeString
	// UnsupportedTypeStringParseError indicates a failure to parse an image string.
	UnsupportedTypeStringParseError
	// UnsupportedTypeNonSourceImage indicates an image string from a non-source registry in strict mode.
	UnsupportedTypeNonSourceImage
	// UnsupportedTypeExcludedImage indicates an image from an explicitly excluded registry.
	UnsupportedTypeExcludedImage
	// UnsupportedTypeList indicates an unsupported list/array structure where an image was expected.
	UnsupportedTypeList
	// UnsupportedTypeMapValue indicates an unsupported value type within a map where an image was expected.
	UnsupportedTypeMapValue
)

// Basic error type for unsupported image structures
type UnsupportedImageError struct {
	Path []string
	Type UnsupportedType
	Err  error
}

func (e *UnsupportedImageError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("unsupported image structure at path %v (type %d): %v", e.Path, e.Type, e.Err)
	}
	return fmt.Sprintf("unsupported image structure at path %v (type %d)", e.Path, e.Type)
}

// Basic constructor for UnsupportedImageError
func NewUnsupportedImageError(path []string, uType UnsupportedType, err error) error {
	return &UnsupportedImageError{Path: path, Type: uType, Err: err}
}
