package helm

// Values represents the hierarchical structure of Helm values, typically loaded
// from a values.yaml file. It uses a map where keys are strings and values can
// be of any type, allowing for nested structures.
type Values map[string]interface{}

// Overrides represents a flat map of image overrides. The keys are the original
// image identifiers (e.g., 'image.repository'), and the values are the new
// image URIs to substitute.
type Overrides map[string]string
