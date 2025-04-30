package helm

// ValueOrigin represents the source of a Helm value.
type ValueOrigin struct {
	// Type identifies the type of value source
	Type ValueOriginType
	// ChartName is the name of the chart that the value comes from (for chart default values)
	ChartName string
	// Path is the file path or flag that provided the value
	Path string
}

// ValueOriginType identifies the source type of a Helm value.
type ValueOriginType string

const (
	// OriginChartDefault indicates the value came from a chart's default values.yaml
	OriginChartDefault ValueOriginType = "chart-default"
	// OriginUserFile indicates the value came from a user-provided values file
	OriginUserFile ValueOriginType = "user-file"
	// OriginUserSet indicates the value came from a --set flag
	OriginUserSet ValueOriginType = "user-set"
	// OriginParentValues indicates the value came from a parent chart's values
	OriginParentValues ValueOriginType = "parent-values"
	// OriginGlobal indicates the value came from global values
	OriginGlobal ValueOriginType = "global"
	// OriginAlias indicates the value came through a chart alias
	OriginAlias ValueOriginType = "alias"
)

// ValueWithOrigin wraps a value with information about its origin
type ValueWithOrigin struct {
	Value  interface{}
	Origin ValueOrigin
}

// CoalescedValues represents the final merged values with origin information
type CoalescedValues struct {
	// Values contains the final merged values
	Values map[string]interface{}
	// Origins tracks the origin of each value path
	Origins map[string]ValueOrigin
}
