package helm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/strvals"
)

const (
	// setFilePartsExpected is the expected number of parts when splitting a --set-file value
	setFilePartsExpected = 2
)

// ChartLoaderOptions contains the options for loading a chart.
type ChartLoaderOptions struct {
	// ChartPath is the path to the chart directory or .tgz file
	ChartPath string

	// ValuesOptions contains values flag options
	ValuesOpts values.Options
}

// ChartLoader is an interface for loading charts and computing values.
type ChartLoader interface {
	// LoadChartWithValues loads a chart and computes its values.
	LoadChartWithValues(opts *ChartLoaderOptions) (*chart.Chart, map[string]interface{}, error)

	// LoadChartAndTrackOrigins loads a chart and computes its values with origin tracking.
	LoadChartAndTrackOrigins(opts *ChartLoaderOptions) (*ChartAnalysisContext, error)
}

// DefaultChartLoader is the default implementation of ChartLoader.
type DefaultChartLoader struct{}

// NewChartLoader creates a new DefaultChartLoader.
func NewChartLoader() ChartLoader {
	return &DefaultChartLoader{}
}

// LoadChartWithValues implements ChartLoader.LoadChartWithValues.
func (l *DefaultChartLoader) LoadChartWithValues(opts *ChartLoaderOptions) (*chart.Chart, map[string]interface{}, error) {
	// Load the chart
	loadedChart, err := loader.Load(opts.ChartPath)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to load chart")
	}

	// Process values options and get user values
	userValues, err := processValuesOptions(&opts.ValuesOpts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to process values")
	}

	// Merge chart values with user values
	mergedValues, err := chartutil.CoalesceValues(loadedChart, userValues)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to coalesce values")
	}

	return loadedChart, mergedValues, nil
}

// LoadChartAndTrackOrigins implements ChartLoader.LoadChartAndTrackOrigins.
func (l *DefaultChartLoader) LoadChartAndTrackOrigins(opts *ChartLoaderOptions) (*ChartAnalysisContext, error) {
	// Load the chart
	loadedChart, err := loader.Load(opts.ChartPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart")
	}

	// Create origins tracking map
	origins := make(map[string]ValueOrigin)

	// Track chart default values
	trackChartDefaultValues(loadedChart, origins, "")

	// Process user-provided values
	userValues := map[string]interface{}{}

	// Process values files
	for _, file := range opts.ValuesOpts.ValueFiles {
		if err := mergeUserValuesFileWithOrigin(file, userValues, origins); err != nil {
			return nil, errors.Wrapf(err, "failed to merge values file %s", file)
		}
	}

	// Process set values
	for _, val := range opts.ValuesOpts.Values {
		if err := applySetValueWithOrigin(val, userValues, origins); err != nil {
			return nil, errors.Wrapf(err, "failed to apply set value %s", val)
		}
	}

	// Process stringValues
	for _, val := range opts.ValuesOpts.StringValues {
		if err := applySetValueWithOrigin(val, userValues, origins); err != nil {
			return nil, errors.Wrapf(err, "failed to apply string value %s", val)
		}
	}

	// Process fileValues
	for _, val := range opts.ValuesOpts.FileValues {
		if err := applySetValueWithOrigin(val, userValues, origins); err != nil {
			return nil, errors.Wrapf(err, "failed to apply file value %s", val)
		}
	}

	// Merge chart values with user values
	mergedValues, err := chartutil.CoalesceValues(loadedChart, userValues)
	if err != nil {
		return nil, errors.Wrap(err, "failed to coalesce values")
	}

	// Create context
	return NewChartAnalysisContext(
		loadedChart,
		mergedValues,
		origins,
		opts.ValuesOpts.ValueFiles,
		append(append(opts.ValuesOpts.Values, opts.ValuesOpts.StringValues...), opts.ValuesOpts.FileValues...),
	), nil
}

// processValuesOptions processes values options and returns a merged map.
func processValuesOptions(valuesOpts *values.Options) (map[string]interface{}, error) {
	base := map[string]interface{}{}

	// Process values files
	for _, filePath := range valuesOpts.ValueFiles {
		currentMap := map[string]interface{}{}

		// Validate file exists
		if _, err := os.Stat(filePath); err != nil {
			return nil, errors.Errorf("values file %q not accessible: %s", filePath, err)
		}

		// Read and parse the file
		// G304: Potential file inclusion vulnerability - filePath needs validation.
		bytes, err := os.ReadFile(filePath) //nolint:gosec // TODO: Validate filePath before reading to prevent reading arbitrary files.
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read values file %s", filePath)
		}

		if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
			return nil, errors.Wrapf(err, "failed parsing values file %s", filePath)
		}

		// Merge with base values
		base = chartutil.CoalesceTables(base, currentMap)
	}

	// Process --set values
	for _, value := range valuesOpts.Values {
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, errors.Wrapf(err, "failed parsing --set %s", value)
		}
	}

	// Process --set-string values
	for _, value := range valuesOpts.StringValues {
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, errors.Wrapf(err, "failed parsing --set-string %s", value)
		}
	}

	// Process --set-file values
	for _, value := range valuesOpts.FileValues {
		// Split the key-value pair
		parts := strings.SplitN(value, "=", setFilePartsExpected)
		if len(parts) != setFilePartsExpected {
			return nil, errors.Errorf("invalid set file value: %s", value)
		}

		// Read file content
		content, err := os.ReadFile(parts[1])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read file %s", parts[1])
		}

		// Set the value to the file content (as string)
		if err := strvals.ParseInto(parts[0]+"="+string(content), base); err != nil {
			return nil, errors.Wrapf(err, "failed parsing --set-file %s", value)
		}
	}

	return base, nil
}

// trackChartDefaultValues recursively records origin information for chart default values.
func trackChartDefaultValues(c *chart.Chart, origins map[string]ValueOrigin, parentPath string) {
	// Track origin for this chart's values.yaml
	if c.Values != nil {
		flattenAndTrackValues(c.Values, origins, ValueOrigin{
			Type:      OriginChartDefault,
			ChartName: c.Name(), // Use the current chart's name
			Path:      filepath.Join(parentPath, "values.yaml"),
		}, "")
	}

	// Recursively process subcharts
	for _, subchart := range c.Dependencies() {
		// Determine the path prefix for values within this subchart
		// Helm merges subchart values under a key matching the subchart's name or alias.
		// We need to reflect this structure when tracking origins.
		// Note: Helm's alias mechanism isn't directly accessible here easily,
		// but CoalesceValues handles the merging correctly later.
		// Our origin tracking primarily needs the correct *source* chart name.
		subchartValuePrefix := subchart.Name() // Use subchart name as the key prefix

		// Create a *new* origin struct specific to this subchart's defaults
		subchartOrigin := ValueOrigin{
			Type:      OriginChartDefault,
			ChartName: subchart.Name(),                                                     // Correctly set the subchart name
			Path:      filepath.Join(parentPath, "charts", subchart.Name(), "values.yaml"), // Path within the chart structure
		}

		// Flatten and track the subchart's default values under its prefix
		if subchart.Values != nil {
			flattenAndTrackValues(subchart.Values, origins, subchartOrigin, subchartValuePrefix)
		}

		// Recursively process the dependencies of the subchart
		// Pass the correct arguments (chart, origins, new parent path)
		trackChartDefaultValues(subchart, origins, filepath.Join(parentPath, "charts", subchart.Name())) // Corrected arguments
	}
}

// flattenAndTrackValues recursively flattens a values map and tracks origins.
// The prefix parameter indicates the path hierarchy (e.g., "childChart.key").
func flattenAndTrackValues(valuesMap map[string]interface{}, origins map[string]ValueOrigin, origin ValueOrigin, prefix string) {
	for k, v := range valuesMap {
		keyPath := k
		if prefix != "" {
			keyPath = prefix + "." + k
		}

		// Only update origin if this path hasn't been set by a higher precedence source
		// (like user values or parent chart defaults). Helm coalescing rules apply.
		// The current simple implementation overwrites; a more sophisticated one might check.
		// However, since we call CoalesceValues later, the *final* values map reflects precedence.
		// Our goal here is to record the *initial* source before CoalesceValues merges.
		// A potential issue: If CoalesceValues picks a parent value over a subchart default,
		// our 'origins' map might incorrectly show the subchart origin if tracked last.
		// TODO: Revisit origin tracking *after* CoalesceValues for ultimate accuracy,
		// although this might be significantly more complex. For now, track initial sources.
		origins[keyPath] = origin

		// Recursively process nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			flattenAndTrackValues(nestedMap, origins, origin, keyPath)
		}
		// TODO: Handle lists/arrays if necessary? Helm treats them mostly as atomic values during merge.
	}
}

// mergeUserValuesFileWithOrigin merges values from a user-provided file and tracks their origin.
func mergeUserValuesFileWithOrigin(fileName string, valuesMap map[string]interface{}, origins map[string]ValueOrigin) error {
	// Read and parse the file
	// G304: Potential file inclusion vulnerability - fileName needs validation.
	bytes, err := os.ReadFile(fileName) //nolint:gosec // Assume validation happens elsewhere for now.
	if err != nil {
		return errors.Wrapf(err, "failed to read values file %s", fileName)
	}

	var fileValues map[string]interface{}
	if err := yaml.Unmarshal(bytes, &fileValues); err != nil {
		return errors.Wrapf(err, "failed to parse values file %s", fileName)
	}

	// Track origins for these values
	origin := ValueOrigin{
		Type: OriginUserFile,
		Path: fileName,
		// ChartName is not applicable for user files
	}
	// User files apply at the root level, so no prefix needed for flattenAndTrackValues
	flattenAndTrackValues(fileValues, origins, origin, "")

	// Merge with existing values (mutates the 'values' map)
	chartutil.CoalesceTables(valuesMap, fileValues)
	return nil
}

// applySetValueWithOrigin applies a --set value and tracks its origin.
func applySetValueWithOrigin(setValue string, valuesMap map[string]interface{}, origins map[string]ValueOrigin) error {
	// Parse the key=value string first to get the key path
	key, _, err := parseSetKey(setValue) // Helper to extract just the key needed for origin map, ignore value
	if err != nil {
		return errors.Wrapf(err, "failed parsing key from set value %s", setValue)
	}

	// Record the origin *before* applying the value, as ParseInto mutates
	origins[key] = ValueOrigin{
		Type: OriginUserSet,
		Path: setValue, // Store the original "key=value" string
		// ChartName is not applicable for --set
	}

	// Apply the set value (mutates the 'values' map)
	if err := strvals.ParseInto(setValue, valuesMap); err != nil {
		// If parsing fails, potentially remove the optimistic origin entry? Or leave it?
		// Leaving it might be confusing if the value wasn't actually applied.
		// Let's remove it for consistency.
		delete(origins, key)
		return errors.Wrapf(err, "failed to parse set value %s", setValue)
	}

	// TODO: Handle complex --set paths like list indices (key[0]=val) if needed.
	// flattenAndTrackValues might need adjustments for lists if we track sub-elements.
	// For now, tracking the top-level key assigned by --set.

	return nil
}

// Helper function to extract the key part of a "key=value" string
func parseSetKey(setValue string) (key, value string, err error) {
	idx := strings.IndexRune(setValue, '=')
	if idx < 0 {
		// Handle --set flags without '=' (e.g., --set key=null or boolean toggles if supported by strvals)
		// For origin tracking, we still need a key. Assume the whole string is the key if no '='.
		// This might need refinement based on exactly how strvals handles valueless keys.
		// Let's require '=' for now for simplicity and safety.
		return "", "", fmt.Errorf("invalid set value %q: must be in key=value format", setValue)
	}
	key = setValue[:idx]
	value = setValue[idx+1:]
	return key, value, nil
}

// END OF FILE - Ensure no other definitions of these functions exist below.
