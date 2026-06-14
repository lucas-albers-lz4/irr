// Package keys defines shared string constants for YAML/Helm field names and formats.
package keys

// Common YAML/Helm field names for container image override structures.
const (
	Registry   = "registry"
	Repository = "repository"
	Tag        = "tag"
	Digest     = "digest"
	Image      = "image"
	PullPolicy = "pullPolicy"

	ValuesYAML = "values.yaml"
	Values     = "values"
	JSON       = "json"
	HelmSet    = "helm-set"

	IfNotPresent = "IfNotPresent"

	MapType    = "map"
	StringType = "string"
)
