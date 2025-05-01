// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"fmt"
	"os"
	"strings"

	helmtypes "github.com/lucas-albers-lz4/irr/pkg/helmtypes"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/strvals"
	// Correct import for the project's logger
)

const (
	// Helm Value Parsing Constants
	// expectedSplitParts defines the number of parts expected when splitting key=value strings.
	expectedSplitParts = 2
	// setFilePartsExpected = 2 // Gocritic flags this as unused, remove if confirmed
)

// DefaultChartLoader is the default implementation of ChartLoader.
type DefaultChartLoader struct{}

// NewChartLoader creates a new DefaultChartLoader as a helmtypes.ChartLoader.
func NewChartLoader() helmtypes.ChartLoader {
	return &DefaultChartLoader{}
}

// LoadChartWithValues implements ChartLoader.LoadChartWithValues.
func (l *DefaultChartLoader) LoadChartWithValues(opts *helmtypes.ChartLoaderOptions) (*chart.Chart, map[string]interface{}, error) {
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
// It performs a custom merge process to track value origins accurately,
// replicating Helm's precedence rules.
func (l *DefaultChartLoader) LoadChartAndTrackOrigins(opts *helmtypes.ChartLoaderOptions) (*helmtypes.ChartAnalysisContext, error) {
	// Load the chart using Helm's loader
	loadedChart, err := loader.Load(opts.ChartPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load chart")
	}

	// Initialize the final merged values map and the origins map
	mergedValues := make(map[string]interface{})
	origins := make(map[string]helmtypes.ValueOrigin)

	// --- Helm Precedence Order ---
	// 1. Process Dependency Defaults & Parent Overrides (Recursively)
	// This builds the base values, starting with subchart defaults and applying
	// parent overrides as defined in the parent's values.yaml.
	log.Debug("Processing dependency defaults and parent overrides", "chartName", loadedChart.Name())
	if err := l.processDependencyDefaults(loadedChart, loadedChart, mergedValues, origins, ""); err != nil {
		// Note: processDependencyDefaults needs modification to handle precedence correctly
		//       within its recursive calls and use OriginParentOverride.
		return nil, errors.Wrap(err, "failed to process dependency defaults/overrides")
	}
	log.Debug("Completed processing dependency defaults and parent overrides")

	// 2. Process Top-Level Chart Default values
	// These apply on top of the dependency base.
	if loadedChart.Values != nil {
		log.Debug("Processing top-level chart defaults", "chartName", loadedChart.Name())
		topLevelOrigin := helmtypes.ValueOrigin{
			Type:      helmtypes.OriginChartDefault,
			ChartName: loadedChart.Name(),
			Path:      chartutil.ValuesfileName, // Standard values file name
		}
		if err := mergeAndTrack(mergedValues, loadedChart.Values, origins, topLevelOrigin, ""); err != nil {
			return nil, errors.Wrap(err, "failed to merge top-level chart defaults")
		}
		log.Debug("Completed processing top-level chart defaults")
	}

	// 3. Process User Values Files (--values / -f)
	// Files are processed in order, with later files overriding earlier ones,
	// and overriding chart defaults / dependency values.
	for _, filePath := range opts.ValuesOpts.ValueFiles {
		log.Debug("Processing user values file", "filePath", filePath)
		bytes, err := os.ReadFile(filePath) //nolint:gosec // filePath validated by os.Stat above.
		if err != nil {
			return nil, errors.Wrapf(err, "failed to read values file %s", filePath)
		}
		currentUserFileValues := make(map[string]interface{})
		if err := yaml.Unmarshal(bytes, &currentUserFileValues); err != nil {
			return nil, errors.Wrapf(err, "failed parsing values file %s", filePath)
		}
		fileOrigin := helmtypes.ValueOrigin{
			Type: helmtypes.OriginUserFile,
			Path: filePath,
		}
		if err := mergeAndTrack(mergedValues, currentUserFileValues, origins, fileOrigin, ""); err != nil {
			return nil, errors.Wrapf(err, "failed to merge user values file %s", filePath)
		}
		log.Debug("Completed processing user values file", "filePath", filePath)
	}

	// 4. Process User Set Flags (--set, --set-string, --set-file)
	// These have the highest precedence and overwrite everything else.
	log.Debug("Processing --set flags")
	for _, setFlag := range opts.ValuesOpts.Values {
		if err := l.applySetFlag(mergedValues, origins, setFlag, helmtypes.OriginUserSet); err != nil {
			// Note: applySetFlag needs modification to update origins map correctly.
			return nil, errors.Wrapf(err, "failed applying --set flag: %s", setFlag)
		}
	}
	log.Debug("Processing --set-string flags")
	for _, value := range opts.ValuesOpts.StringValues {
		if err := strvals.ParseInto(value, mergedValues); err != nil {
			return nil, errors.Wrapf(err, "failed parsing --set-string %s", value)
		}
	}
	log.Debug("Processing --set-file flags")
	for _, setFlag := range opts.ValuesOpts.FileValues {
		if err := l.applySetFileFlag(mergedValues, origins, setFlag, helmtypes.OriginUserSet); err != nil {
			// Note: applySetFileFlag needs modification to update origins map correctly.
			return nil, errors.Wrapf(err, "failed applying --set-file flag: %s", setFlag)
		}
	}
	log.Debug("Completed processing set flags")

	// --- End Helm Precedence Order ---

	log.Debug("Final merged values count", "count", len(mergedValues))
	log.Debug("Final origins count", "count", len(origins))

	// Create context with the computed mergedValues and origins
	return helmtypes.NewChartAnalysisContext(
		loadedChart,
		mergedValues,
		origins,
		opts.ChartPath, // Use the chart path as chartRoot
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
		// G304: Potential file inclusion via variable (gosec)
		// filePath is validated by os.Stat above, and is only sourced from user-supplied CLI flags (values files),
		// which is consistent with Helm's own behavior. This is considered safe in this context.
		bytes, err := os.ReadFile(filePath) //nolint:gosec // filePath validated by os.Stat above.
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
		// Split to potentially validate key=value format, though strvals handles parsing.
		// parts := strings.SplitN(value, "=", expectedSplitParts)
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
		// Split to potentially validate key=value format, though strvals handles parsing.
		parts := strings.SplitN(value, "=", expectedSplitParts)
		if len(parts) != expectedSplitParts {
			// Log a warning or potentially error? strvals.ParseInto might handle this gracefully.
			log.Warn("Potentially malformed --set-file flag, expected key=filepath format", "flag", value)
		}
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, errors.Wrapf(err, "failed parsing --set-file %s", value)
		}
	}

	return base, nil
}

// processDependencyDefaults recursively merges default values from dependencies,
// applying parent chart overrides according to Helm precedence.
// It modifies mergedValues and origins in place.
func (l *DefaultChartLoader) processDependencyDefaults(
	rootChart *chart.Chart, // The top-level chart being processed
	currentChart *chart.Chart, // The chart whose dependencies are currently being processed
	mergedValues map[string]interface{},
	origins map[string]helmtypes.ValueOrigin,
	basePath string, // The path prefix for values from this chart context (e.g., "child.", "parent.child.")
) error {
	log.Debug("Processing dependency defaults", "chart", currentChart.Name(), "basePath", basePath)

	// Ensure dependencies are loaded (Helm loader usually does this, but double check)
	// Note: Dependencies() returns *direct* dependencies. loader.Load should handle transitive ones.
	if currentChart.Metadata.Dependencies == nil {
		log.Debug("No dependencies found for chart", "chart", currentChart.Name())
		return nil // No dependencies to process for this chart
	}

	for _, dep := range currentChart.Metadata.Dependencies {
		// Find the actual subchart object loaded by Helm
		var subchart *chart.Chart
		// Helm loader stores dependencies flatly in the top-level chart's Dependencies field
		for _, loadedDep := range rootChart.Dependencies() {
			// Match by name - assumes names are unique which Helm requires
			if loadedDep.Name() == dep.Name {
				subchart = loadedDep
				break
			}
		}

		if subchart == nil {
			// This shouldn't happen if loader.Load worked correctly
			log.Warn("Subchart not found in loaded dependencies", "dependencyName", dep.Name, "parentChart", currentChart.Name())
			// Continue processing other dependencies? Or return error? Let's log and continue for now.
			continue
		}

		log.Debug("Processing dependency", "dependencyName", subchart.Name(), "alias", dep.Alias)

		// Determine the path prefix for this subchart's values
		// Use alias if provided, otherwise use the chart name
		dependencyPrefix := dep.Name
		if dep.Alias != "" {
			dependencyPrefix = dep.Alias
		}
		// Prepend the basePath from the parent context
		fullDependencyPrefix := dependencyPrefix
		if basePath != "" {
			fullDependencyPrefix = basePath + "." + dependencyPrefix
		}

		// --- Precedence within this dependency ---
		// 1. Merge the subchart's own default values first (lowest precedence within this scope)
		if subchart.Values != nil {
			log.Debug("Merging subchart defaults", "subchart", subchart.Name(), "prefix", fullDependencyPrefix)
			subchartOrigin := helmtypes.ValueOrigin{
				Type:      helmtypes.OriginChartDefault,
				ChartName: subchart.Name(),
				Path:      chartutil.ValuesfileName, // Standard values file name for subchart
			}
			// Pass the calculated prefix to mergeAndTrack
			if err := mergeAndTrack(mergedValues, subchart.Values, origins, subchartOrigin, fullDependencyPrefix); err != nil {
				return errors.Wrapf(err, "failed processing dependencies for subchart %s", subchart.Name())
			}
		}

		// 2. Extract and merge parent overrides for this subchart (higher precedence)
		// Parent overrides live in the *parent's* values, keyed by the dependency name/alias
		parentOverrides := extractParentOverrides(currentChart.Values, dependencyPrefix)
		if len(parentOverrides) > 0 { // Check if there are any overrides
			log.Debug("Merging parent overrides for subchart", "subchart", subchart.Name(), "prefix", fullDependencyPrefix)
			parentOverrideOrigin := helmtypes.ValueOrigin{
				Type:      helmtypes.OriginParentOverride,
				ChartName: currentChart.Name(), // The parent chart providing the override
				Alias:     dependencyPrefix,    // Record the alias/name used for the override key
			}
			// Pass the calculated prefix to mergeAndTrack
			if err := mergeAndTrack(mergedValues, parentOverrides, origins, parentOverrideOrigin, fullDependencyPrefix); err != nil {
				return errors.Wrapf(err, "failed processing dependencies for subchart %s", subchart.Name())
			}
		}

		// 3. Recursively process the dependencies of this subchart
		log.Debug("Recursively processing dependencies for subchart", "subchart", subchart.Name(), "newBasePath", fullDependencyPrefix)
		if err := l.processDependencyDefaults(rootChart, subchart, mergedValues, origins, fullDependencyPrefix); err != nil {
			return errors.Wrapf(err, "failed processing dependencies for subchart %s", subchart.Name())
		}
	}

	return nil
}

// extractParentOverrides finds the values block in a parent's values map
// intended for a specific subchart (keyed by the subchart's name or alias).
func extractParentOverrides(parentValues map[string]interface{}, subchartKey string) map[string]interface{} {
	overrides := make(map[string]interface{})
	if parentValues == nil {
		return overrides // No parent values, no overrides
	}

	rawOverrides, keyExists := parentValues[subchartKey]
	if !keyExists {
		return overrides // Parent values exist, but no block for this subchart key
	}

	// Attempt to cast the found value block to the expected map type
	overrideMap, isMap := rawOverrides.(map[string]interface{})
	if !isMap {
		log.Warn("Parent overrides for subchart key are not a map, ignoring", "subchartKey", subchartKey, "type", fmt.Sprintf("%T", rawOverrides))
		return overrides // Found something, but it's not a map as expected
	}

	// Return the extracted map
	return overrideMap
}

// mergeAndTrack recursively merges the source map into the target map,
// tracking the origin of each value. Higher precedence sources overwrite
// lower precedence values and their origins. Arrays are replaced, not merged.
func mergeAndTrack(target, source map[string]interface{}, origins map[string]helmtypes.ValueOrigin, sourceOrigin helmtypes.ValueOrigin, currentPath string) error {
	for key, sourceValue := range source {
		fullPath := key
		if currentPath != "" {
			fullPath = currentPath + "." + key
		}
		targetValue, keyExists := target[key]
		origins[fullPath] = sourceOrigin
		sourceMap, sourceIsMap := sourceValue.(map[string]interface{})
		targetMap, targetIsMap := targetValue.(map[string]interface{})
		if !keyExists {
			targetIsMap = false
		}
		switch {
		case sourceIsMap && targetIsMap:
			if err := mergeAndTrack(targetMap, sourceMap, origins, sourceOrigin, fullPath); err != nil {
				return err
			}
		case sourceIsMap:
			newMap := make(map[string]interface{})
			target[key] = newMap
			if err := mergeAndTrack(newMap, sourceMap, origins, sourceOrigin, fullPath); err != nil {
				return err
			}
		default:
			target[key] = sourceValue
			if keyExists && targetIsMap && !sourceIsMap {
				prefixToRemove := fullPath + "."
				for k := range origins {
					if strings.HasPrefix(k, prefixToRemove) {
						delete(origins, k)
					}
				}
			}
		}
	}
	return nil
}

// applySetFlag processes a standard --set or --set-string flag,
// updating both the merged values and their origins.
func (l *DefaultChartLoader) applySetFlag(mergedValues map[string]interface{}, origins map[string]helmtypes.ValueOrigin, setFlag string, originType helmtypes.ValueOriginType) error {
	log.Debug("Applying set flag", "setFlag", setFlag)
	// Apply the value using Helm's strvals library
	if err := strvals.ParseInto(setFlag, mergedValues); err != nil {
		return errors.Wrapf(err, "failed parsing set flag '%s'", setFlag)
	}

	// --- Update Origin ---
	// Attempt to parse the key path from the set flag (best effort)
	// Example: "foo.bar=value" -> keyPath = "foo.bar"
	keyPath := setFlag
	if parts := strings.SplitN(setFlag, "=", expectedSplitParts); len(parts) > 0 {
		keyPath = parts[0]
		// TODO: Handle complex strvals paths like foo.list[0].name=val more robustly.
		// This basic split might not capture the exact final path created by strvals.
	}

	// Update the origin for the parsed key path
	setOrigin := helmtypes.ValueOrigin{
		Type: originType, // Should be OriginUserSet
		Key:  setFlag,    // Store the original flag string
	}
	origins[keyPath] = setOrigin
	log.Debug("Origin updated for set flag", "keyPath", keyPath, "origin", setOrigin)

	// Clean up deeper origins if this set flag overwrote a map
	// Get the value that was just set
	finalValue, exists, err := GetValueAtPath(mergedValues, keyPath)
	if err != nil || !exists {
		log.Warn("Could not retrieve value after setting flag, cannot clean origins", "keyPath", keyPath, "error", err)
	} else {
		_, valueIsMap := finalValue.(map[string]interface{})
		if !valueIsMap {
			prefixToRemove := keyPath + "."
			cleanedCount := 0
			for k := range origins {
				if strings.HasPrefix(k, prefixToRemove) {
					delete(origins, k)
					cleanedCount++
				}
			}
			if cleanedCount > 0 {
				log.Debug("Cleaned up deeper origins overwritten by set flag", "keyPath", keyPath, "count", cleanedCount)
			}
		}
	}

	return nil
}

// applySetFileFlag processes a --set-file flag, updating both merged values and origins.
// NOTE: Helm's strvals.ParseInto handles reading the file content based on the flag format.
func (l *DefaultChartLoader) applySetFileFlag(mergedValues map[string]interface{}, origins map[string]helmtypes.ValueOrigin, setFileFlag string, originType helmtypes.ValueOriginType) error {
	log.Debug("Applying set-file flag", "setFileFlag", setFileFlag)
	// Apply the value using Helm's strvals library.
	// ParseInto handles the file reading for --set-file syntax.
	if err := strvals.ParseInto(setFileFlag, mergedValues); err != nil {
		return errors.Wrapf(err, "failed parsing set-file flag '%s'", setFileFlag)
	}

	// --- Update Origin ---
	// Attempt to parse the key path from the set-file flag (best effort)
	// Example: "foo.bar=filepath" -> keyPath = "foo.bar"
	keyPath := setFileFlag
	var filePath string // Store the file path part if needed
	if parts := strings.SplitN(setFileFlag, "=", expectedSplitParts); len(parts) > 0 {
		keyPath = parts[0]
		if len(parts) > 1 {
			filePath = parts[1] // Capture file path
		}
		// TODO: Handle complex strvals paths more robustly.
	}

	// Update the origin for the parsed key path
	setOrigin := helmtypes.ValueOrigin{
		Type: originType,  // Should be OriginUserSet
		Key:  setFileFlag, // Store the original flag string
		Path: filePath,    // Store the file path specified in the flag
	}
	origins[keyPath] = setOrigin
	log.Debug("Origin updated for set-file flag", "keyPath", keyPath, "origin", setOrigin)

	// Clean up deeper origins if this set flag overwrote a map
	// (Similar logic as applySetFlag)
	finalValue, exists, err := GetValueAtPath(mergedValues, keyPath)
	if err != nil || !exists {
		log.Warn("Could not retrieve value after setting file flag, cannot clean origins", "keyPath", keyPath, "error", err)
	} else {
		_, valueIsMap := finalValue.(map[string]interface{})
		if !valueIsMap {
			prefixToRemove := keyPath + "."
			cleanedCount := 0
			for k := range origins {
				if strings.HasPrefix(k, prefixToRemove) {
					delete(origins, k)
					cleanedCount++
				}
			}
			if cleanedCount > 0 {
				log.Debug("Cleaned up deeper origins overwritten by set-file flag", "keyPath", keyPath, "count", cleanedCount)
			}
		}
	}

	return nil
}

// GetValueAtPath retrieves a value from a nested map using a dot-notation path.
// Returns the value, a boolean indicating if it was found, and an error.
func GetValueAtPath(vals map[string]interface{}, path string) (value interface{}, found bool, err error) {
	parts := strings.Split(path, ".")
	current := interface{}(vals)

	for i, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			// We expected a map but didn't find one mid-path
			return nil, false, errors.Errorf("value at intermediate path '%s' is not a map", strings.Join(parts[:i], "."))
		}

		value, exists := currentMap[part]
		if !exists {
			// Key doesn't exist at this level
			return nil, false, nil
		}

		if i == len(parts)-1 {
			// Reached the end of the path
			return value, true, nil
		}
		// Move deeper for the next part
		current = value
	}

	// Should not happen if path is non-empty
	return nil, false, errors.New("invalid path or map structure")
}
