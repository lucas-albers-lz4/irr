package override

import (
	"errors"
	"fmt"
)

// Override package errors.
var (
	// ErrNilDataMap is returned when attempting to set a value on a nil map.
	ErrNilDataMap = errors.New("data map cannot be nil")

	// ErrEmptyPath is returned when an empty path is provided.
	ErrEmptyPath = errors.New("empty path")

	// ErrNilImageReference is returned when attempting to override with a nil image reference.
	ErrNilImageReference = errors.New("nil image reference")

	// ErrPathParsing is returned when a path part cannot be parsed.
	ErrPathParsing = errors.New("error parsing path part")

	// ErrNegativeArrayIndex is returned when a negative array index is provided.
	ErrNegativeArrayIndex = errors.New("negative array index")

	// ErrNotAnArray is returned when trying to access an array index on a non-array value.
	ErrNotAnArray = errors.New("value is not an array")

	// ErrNonMapTraversal is returned when trying to traverse through a non-map value.
	ErrNonMapTraversal = errors.New("cannot traverse through non-map")

	// ErrInvalidArrayIndex is returned when an invalid array index is provided.
	ErrInvalidArrayIndex = errors.New("invalid non-integer array index")

	// ErrMalformedArrayIndex is returned when array index syntax is malformed.
	ErrMalformedArrayIndex = errors.New("malformed array index syntax")

	// ErrMarshalOverrides is returned when overrides cannot be marshaled to YAML.
	ErrMarshalOverrides = errors.New("failed to marshal overrides to YAML")

	// ErrJSONToYAML is returned when JSON cannot be converted to YAML.
	ErrJSONToYAML = errors.New("failed to convert JSON to YAML")
)

// WrapPathParsing wraps ErrPathParsing with the given path part and error for context.
func WrapPathParsing(part string, err error) error {
	return fmt.Errorf("%w: '%s': %w", ErrPathParsing, part, err)
}

// WrapNegativeArrayIndex wraps ErrNegativeArrayIndex with the given index for context.
func WrapNegativeArrayIndex(index int) error {
	return fmt.Errorf("%w: %d", ErrNegativeArrayIndex, index)
}

// WrapNotAnArray wraps ErrNotAnArray with the given key for context.
func WrapNotAnArray(key string) error {
	return fmt.Errorf("%w: path element %s exists but is not an array", ErrNotAnArray, key)
}

// WrapNonMapTraversalArray wraps ErrNonMapTraversal with array index context.
func WrapNonMapTraversalArray(index int, value interface{}) error {
	return fmt.Errorf("%w at index %d which holds value %T", ErrNonMapTraversal, index, value)
}

// WrapNonMapTraversalMap wraps ErrNonMapTraversal with map key context.
func WrapNonMapTraversalMap(key string) error {
	return fmt.Errorf("%w at key %s", ErrNonMapTraversal, key)
}

// WrapInvalidArrayIndex wraps ErrInvalidArrayIndex with the given index string and path part for context.
func WrapInvalidArrayIndex(indexStr, part string) error {
	return fmt.Errorf("%w: '%s' in path part '%s'", ErrInvalidArrayIndex, indexStr, part)
}

// WrapMalformedArrayIndex wraps ErrMalformedArrayIndex with the given path part for context.
func WrapMalformedArrayIndex(part string) error {
	return fmt.Errorf("%w in path part '%s'", ErrMalformedArrayIndex, part)
}

// WrapMarshalOverrides wraps ErrMarshalOverrides with the original error for context.
func WrapMarshalOverrides(err error) error {
	return fmt.Errorf("%w: %w", ErrMarshalOverrides, err)
}

// WrapJSONToYAML wraps ErrJSONToYAML with the original error for context.
func WrapJSONToYAML(err error) error {
	return fmt.Errorf("%w: %w", ErrJSONToYAML, err)
}
