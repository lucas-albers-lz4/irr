// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"os"
	"strings"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
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
	// OriginUserFileSet indicates a value set by --set-file
	OriginUserFileSet ValueOriginType = "user-set-file" // Origin from --set-file
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

	// 1. Process USER-PROVIDED values into userValues map
	userValues, err := processUserProvidedValues(opts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process user provided values")
	}

	// 2. Merge chart default values with processed user values to get FINAL structure
	log.Debug("LoadChartAndTrackOrigins: Coalescing final values...")
	mergedValues, err := chartutil.CoalesceValues(loadedChart, userValues)
	if err != nil {
		return nil, errors.Wrap(err, "failed to coalesce final values")
	}
	log.Debug("LoadChartAndTrackOrigins: Final merged values structure obtained (before alias correction)", "keys", mapKeys(mergedValues))

	// 3. Track Origins based on precedence (User > Parent Default > Subchart Default)
	origins, err := trackValueOrigins(loadedChart, opts, userValues, mergedValues)
	if err != nil {
		return nil, errors.Wrap(err, "failed to track value origins")
	}

	// 4. Perform Alias Correction on FINAL mergedValues map
	log.Debug("LoadChartAndTrackOrigins: Starting alias correction on final merged values...")
	correctedMergedValues := applyAliasCorrection(loadedChart, mergedValues)
	log.Debug("LoadChartAndTrackOrigins: Finished alias correction.")
	finalKeys := []string{}
	for k := range correctedMergedValues {
		finalKeys = append(finalKeys, k)
	}
	log.Debug("LoadChartAndTrackOrigins: Final keys in corrected merged values", "keys", finalKeys)

	// 5. Create context with final values and origins
	log.Debug("LoadChartAndTrackOrigins: Final keys in origins map before return", "keys", mapKeysFromOrigin(origins))
	return NewChartAnalysisContext(
		loadedChart,
		correctedMergedValues, // Use the alias-corrected map
		origins,               // Use the layered origins map
		opts.ValuesOpts.ValueFiles,
		append(append(opts.ValuesOpts.Values, opts.ValuesOpts.StringValues...), opts.ValuesOpts.FileValues...),
	), nil
}

// processUserProvidedValues extracts user-provided values from options.
func processUserProvidedValues(opts *ChartLoaderOptions) (map[string]interface{}, error) {
	log.Debug("processUserProvidedValues: Processing user-provided values...")
	userValues := map[string]interface{}{}
	// Process values files
	for _, file := range opts.ValuesOpts.ValueFiles {
		if err := mergeUserValuesFileWithOrigin(file, userValues, nil); err != nil { // Pass nil for origins
			return nil, errors.Wrapf(err, "failed to merge values file %s", file)
		}
	}
	// Process set values
	for _, val := range opts.ValuesOpts.Values {
		if err := applySetValueWithOrigin(val, userValues, nil); err != nil { // Pass nil for origins
			return nil, errors.Wrapf(err, "failed to apply set value %s", val)
		}
	}
	// Process stringValues
	for _, val := range opts.ValuesOpts.StringValues {
		if err := applySetValueWithOrigin(val, userValues, nil); err != nil { // Pass nil for origins
			return nil, errors.Wrapf(err, "failed to apply string value %s", val)
		}
	}
	// Process fileValues
	for _, val := range opts.ValuesOpts.FileValues {
		key, fileContent, err := parseFileSet(val)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse file value %s", val)
		}
		if err := strvals.ParseInto(key+"="+fileContent, userValues); err != nil {
			return nil, errors.Wrapf(err, "failed applying file value content for %s", key)
		}
	}
	log.Debug("processUserProvidedValues: Finished processing user-provided values", "keys", mapKeys(userValues))
	return userValues, nil
}

// trackValueOrigins tracks origins based on precedence.
func trackValueOrigins(loadedChart *chart.Chart, opts *ChartLoaderOptions, _, _ map[string]interface{}) (map[string]ValueOrigin, error) {
	log.Debug("trackValueOrigins: Starting origin tracking...")
	origins := make(map[string]ValueOrigin)

	// Track User File Origins
	log.Debug("trackValueOrigins: Tracking origins from user files...")
	for _, file := range opts.ValuesOpts.ValueFiles {
		bytes, err := os.ReadFile(file) //nolint:gosec // filePath is validated earlier in the process
		if err != nil {
			return nil, errors.Wrapf(err, "failed to re-read values file %s for origin tracking", file)
		}
		var fileValues map[string]interface{}
		if err := yaml.Unmarshal(bytes, &fileValues); err != nil {
			return nil, errors.Wrapf(err, "failed to re-parse values file %s for origin tracking", file)
		}
		forceFlattenAndTrackOrigins(fileValues, origins, ValueOrigin{Type: OriginUserFile, Path: file}, "")
	}

	// Track User --set Origins
	log.Debug("trackValueOrigins: Tracking origins from --set values...")
	// Recombine set values for iteration (appendAssign fix applied here requires this approach)
	allSetValues := make([]string, 0, len(opts.ValuesOpts.Values)+len(opts.ValuesOpts.StringValues))
	allSetValues = append(allSetValues, opts.ValuesOpts.Values...)
	allSetValues = append(allSetValues, opts.ValuesOpts.StringValues...)
	for _, val := range allSetValues {
		key, _, err := parseSetKey(val)
		if err != nil {
			return nil, errors.Wrapf(err, "failed parsing key from set value %s for origin tracking", val)
		}
		origins[key] = ValueOrigin{Type: OriginUserSet, Path: val}
		// TODO: Handle nested keys from --set
	}

	// Track User --set-file Origins
	log.Debug("trackValueOrigins: Tracking origins from --set-file values...")
	for _, val := range opts.ValuesOpts.FileValues {
		key, _, err := parseFileSet(val)
		if err != nil {
			return nil, errors.Wrapf(err, "failed parsing key from file value %s for origin tracking", val)
		}
		origins[key] = ValueOrigin{Type: OriginUserFileSet, Path: val}
		// TODO: Handle nested keys from --set-file?
	}

	// Track Parent Default Origins
	log.Debug("trackValueOrigins: Tracking origins from parent defaults...")
	if loadedChart.Values != nil {
		flattenAndTrackValues(loadedChart.Values, origins, ValueOrigin{
			Type: OriginChartDefault, ChartName: loadedChart.Name(), Path: "values.yaml",
		}, "")
	}

	// Track Subchart Default Origins
	log.Debug("trackValueOrigins: Tracking origins from subchart defaults...")
	trackAllSubchartValues(loadedChart, origins, ".")

	log.Debug("trackValueOrigins: Finished origin tracking.")
	return origins, nil
}

// applyAliasCorrection adjusts the merged values map based on dependency aliases.
func applyAliasCorrection(loadedChart *chart.Chart, mergedValues map[string]interface{}) map[string]interface{} {
	correctedMergedValues := make(map[string]interface{})
	processedDependencyKeys := make(map[string]bool)

	if loadedChart != nil && loadedChart.Metadata != nil && loadedChart.Metadata.Dependencies != nil {
		for _, parentDepEntry := range loadedChart.Metadata.Dependencies {
			depName := parentDepEntry.Name
			depAlias := parentDepEntry.Alias
			log.Debug("Alias Correction: Checking dependency", "name", depName, "alias", depAlias)
			if originalValue, exists := mergedValues[depName]; exists {
				log.Debug("Alias Correction: Found key matching dependency name", "key", depName)
				mergeKey := depName
				if depAlias != "" {
					mergeKey = depAlias
					log.Debug("Alias Correction: Using alias as merge key", "alias", mergeKey)
				}
				correctedMergedValues[mergeKey] = originalValue
				processedDependencyKeys[depName] = true
				log.Debug("Alias Correction: Added/Replaced key in corrected map", "key", mergeKey, "processed_original", depName)
			} else {
				log.Debug("Alias Correction: Key matching dependency name NOT found in merged values", "key", depName)
			}
		}
	}

	log.Debug("Alias Correction: Copying remaining non-dependency keys...")
	for k, v := range mergedValues {
		if !processedDependencyKeys[k] {
			correctedMergedValues[k] = v
			log.Debug("Alias Correction: Copied non-dependency key", "key", k)
		}
	}
	return correctedMergedValues
}

// Helper function to parse --set-file argument
func parseFileSet(fileSet string) (key, content string, err error) {
	parts := strings.SplitN(fileSet, "=", setFilePartsExpected)
	if len(parts) != setFilePartsExpected {
		return "", "", fmt.Errorf("invalid --set-file format: %s", fileSet)
	}
	key = parts[0]
	filePath := parts[1]
	bytes, err := os.ReadFile(filePath) //nolint:gosec // filePath is validated earlier in the process
	if err != nil {
		return key, "", errors.Wrapf(err, "failed reading file path %s for --set-file key %s", filePath, key)
	}
	content = string(bytes)
	return key, content, nil
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
		bytes, err := os.ReadFile(filePath) //nolint:gosec // NOTE: Needs validation to prevent reading arbitrary files.
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

// trackAllSubchartValues recursively traverses dependencies and tracks their default values.
func trackAllSubchartValues(parentChart *chart.Chart, origins map[string]ValueOrigin, parentPrefix string) {
	if parentChart == nil || parentChart.Metadata == nil {
		return
	}

	for _, dep := range parentChart.Dependencies() { // Dependencies() returns []*chart.Chart
		if dep == nil || dep.Metadata == nil {
			continue
		}

		// Determine the correct key prefix for this subchart's values
		// ALWAYS use the dependency NAME for tracking default value origins,
		// as aliases are applied later during value merging.
		depPrefix := dep.Name()
		log.Debug("Using name for subchart default value origin prefix", "subchart", dep.Name())

		// Construct the full path prefix for origin tracking
		fullPrefix := depPrefix
		if parentPrefix != "." { // Avoid prefixes like ".child"
			fullPrefix = parentPrefix + "." + depPrefix
		}

		// Track this subchart's default values
		if dep.Values != nil {
			log.Debug("Tracking default values for subchart", "subchart", dep.Name(), "prefix", fullPrefix, "keys", mapKeys(dep.Values))
			flattenAndTrackValues(dep.Values, origins, ValueOrigin{
				Type:      OriginChartDefault,
				ChartName: dep.Name(), // Origin is the subchart itself
				Path:      "values.yaml",
			}, fullPrefix) // Use the calculated prefix
		}

		// Recurse into the subchart's dependencies
		trackAllSubchartValues(dep, origins, fullPrefix)
	}
}

// flattenAndTrackValues recursively flattens a values map and tracks origins.
// It now respects precedence and does NOT overwrite existing origins.
func flattenAndTrackValues(valuesMap map[string]interface{}, origins map[string]ValueOrigin, origin ValueOrigin, prefix string) {
	for k, v := range valuesMap {
		keyPath := k
		if prefix != "" {
			keyPath = prefix + "." + k
		}

		// Only record the origin if this path hasn't been recorded yet.
		// This prioritizes higher-precedence sources (user files, --set) tracked earlier.
		if _, exists := origins[keyPath]; !exists {
			log.Debug("flattenAndTrackValues: Tracking origin for key (first time)", "keyPath", keyPath, "originChart", origin.ChartName, "originType", origin.Type, "originPath", origin.Path)
			origins[keyPath] = origin
		} else {
			log.Debug("flattenAndTrackValues: Skipping origin tracking for key (already exists)", "keyPath", keyPath)
		}

		if recursiveMap, ok := v.(map[string]interface{}); ok {
			flattenAndTrackValues(recursiveMap, origins, origin, keyPath)
		}
	}
}

// mergeUserValuesFileWithOrigin only merges values now, origin tracking happens later.
func mergeUserValuesFileWithOrigin(fileName string, valuesMap map[string]interface{}, _ map[string]ValueOrigin /* origins no longer modified here */) error {
	// Read and parse the file
	// G304: Potential file inclusion vulnerability - fileName needs validation.
	bytes, err := os.ReadFile(fileName) //nolint:gosec // NOTE: Needs validation to prevent reading arbitrary files.
	if err != nil {
		return errors.Wrapf(err, "failed to read values file %s", fileName)
	}

	var fileValues map[string]interface{}
	if err := yaml.Unmarshal(bytes, &fileValues); err != nil {
		return errors.Wrapf(err, "failed to parse values file %s", fileName)
	}

	// Merge with existing values (mutates the 'values' map)
	// Note: CoalesceTables merges fileValues INTO valuesMap
	chartutil.CoalesceTables(valuesMap, fileValues)
	return nil
}

// applySetValueWithOrigin only applies --set value now, origin tracking happens later.
func applySetValueWithOrigin(setValue string, valuesMap map[string]interface{}, _ map[string]ValueOrigin /* origins no longer modified here */) error {
	// Apply the set value (mutates the 'values' map)
	if err := strvals.ParseInto(setValue, valuesMap); err != nil {
		return errors.Wrapf(err, "failed to parse set value %s", setValue)
	}
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

// Helper to get keys from ValueOrigin map
func mapKeysFromOrigin(m map[string]ValueOrigin) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// mapKeys returns the keys of a map[string]interface{}.
func mapKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// forceFlattenAndTrackOrigins is similar to flattenAndTrackValues but *always* sets the origin,
// effectively overwriting any previous origin for the same path. Used for higher-precedence sources.
func forceFlattenAndTrackOrigins(valuesMap map[string]interface{}, origins map[string]ValueOrigin, origin ValueOrigin, prefix string) {
	for k, v := range valuesMap {
		keyPath := k
		if prefix != "" {
			keyPath = prefix + "." + k
		}

		// Always set/overwrite the origin for this path
		log.Debug("forceFlattenAndTrackOrigins: Setting/Overwriting origin", "keyPath", keyPath, "originType", origin.Type, "originPath", origin.Path)
		origins[keyPath] = origin

		// Recursively process nested maps
		if nestedMap, ok := v.(map[string]interface{}); ok {
			// Pass the same origin down for nested structures within this source.
			forceFlattenAndTrackOrigins(nestedMap, origins, origin, keyPath)
		}
	}
}

// END OF FILE - Ensure no other definitions of these functions exist below.
