package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	internalhelm "github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/chart"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/registry"
	"github.com/lucas-albers-lz4/irr/pkg/strategy"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli/values"
)

// deriveSourceRegistriesFromMappings populates the SourceRegistries in the config
// from the Mappings, if SourceRegistries is not already set.
func deriveSourceRegistriesFromMappings(config *GeneratorConfig) {
	if config == nil {
		log.Warn("deriveSourceRegistriesFromMappings called with nil config")
		return
	}

	// If --source-registries flag was set (i.e., config.SourceRegistries is not empty),
	// or if mappings were not loaded, or no mapping entries exist, do nothing.
	switch {
	case len(config.SourceRegistries) > 0:
		log.Debug("Source registries explicitly provided via CLI, not deriving from mappings",
			"count", len(config.SourceRegistries),
			"registries", config.SourceRegistries)
		return
	case config.Mappings == nil:
		log.Debug("No mappings loaded, cannot derive source registries")
		return
	case len(config.Mappings.Entries) == 0:
		log.Debug("Mappings loaded but contain no entries, cannot derive source registries")
		return
	}

	// If we reach here, we need to derive source registries from the mappings
	var sourcesFromMappings []string
	seenSources := make(map[string]bool)

	for _, entry := range config.Mappings.Entries { // Mappings.Entries are already filtered by Enabled due to ToMappings()
		// Normalize source from mapping for consistent matching
		originalSourceFromMapping := entry.Source // Store original for comparison
		normalizedMappingSource := image.NormalizeRegistry(originalSourceFromMapping)

		// Log if normalization changed the source string from the mapping file
		if originalSourceFromMapping != normalizedMappingSource {
			log.Debug("Normalized source registry from mapping file",
				"original", originalSourceFromMapping,
				"normalized", normalizedMappingSource)
		}

		if normalizedMappingSource == "" { // Should not happen with valid config, but defense
			log.Warn("Skipping mapping with empty source registry", "target", entry.Target)
			continue
		}

		if !seenSources[normalizedMappingSource] {
			seenSources[normalizedMappingSource] = true
			sourcesFromMappings = append(sourcesFromMappings, normalizedMappingSource)
		}
	}

	if len(sourcesFromMappings) > 0 {
		log.Info("Derived source registries from registry-file mappings",
			"count", len(sourcesFromMappings),
			"registries", sourcesFromMappings)
		config.SourceRegistries = sourcesFromMappings
	} else {
		log.Debug("No valid source registries could be derived from mappings")
	}
}

// setupGeneratorConfig retrieves and configures all options for the generator
// It ONLY gathers flags and populates the struct. Further processing happens in runOverride.
func setupGeneratorConfig(cmd *cobra.Command, isPluginOperatingOnRelease bool) (config GeneratorConfig, err error) {
	// Determine if a config file is provided, to pass to getRequiredFlags
	registryFilePath, regErr := cmd.Flags().GetString("registry-file")
	if regErr != nil {
		return config, fmt.Errorf("failed to get registry-file flag: %w", regErr)
	}

	deprecatedConfigPath, cfgErr := cmd.Flags().GetString("config")
	if cfgErr != nil {
		return config, fmt.Errorf("failed to get config flag: %w", cfgErr)
	}

	isConfigProvided := registryFilePath != "" || deprecatedConfigPath != ""

	// Get required flags first, now context-aware
	chartPathVal, targetRegistryVal, sourceRegistriesVal, err := getRequiredFlags(cmd, isPluginOperatingOnRelease, isConfigProvided)
	if err != nil {
		return config, err // Return zero config on error
	}
	config.ChartPath = chartPathVal
	config.TargetRegistry = targetRegistryVal
	config.SourceRegistries = sourceRegistriesVal

	// Get optional flags
	excludeRegistries, err := getStringSliceFlag(cmd, "exclude-registries")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.ExcludeRegistries = excludeRegistries

	strictMode, err := getBoolFlag(cmd, "strict")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.StrictMode = strictMode

	includePatterns, excludePatterns, err := getAnalysisControlFlags(cmd)
	if err != nil {
		return config, err // Return zero config on error
	}
	config.IncludePatterns = includePatterns
	config.ExcludePatterns = excludePatterns

	disableRules, err := getBoolFlag(cmd, "disable-rules")
	if err != nil {
		return config, err // Return zero config on error
	}
	config.RulesEnabled = !disableRules

	// NOTE: We do NOT call setupPathStrategy, loadRegistryMappings, logConfigMode,
	// or validateUnmappableRegistries here. They are called in runOverride
	// after this function returns successfully.

	// Log excluded registries if any were provided
	if len(config.ExcludeRegistries) > 0 {
		log.Info("Excluding registries", "registries", strings.Join(config.ExcludeRegistries, ", "))
	}

	// Successfully gathered all flags
	return config, nil
}

// setupPathStrategy initializes and validates the path strategy.
func setupPathStrategy(config *GeneratorConfig) (strategy.PathStrategy, error) {
	if config == nil {
		return nil, errors.New("nil config in setupPathStrategy")
	}
	// Default to prefix-source-registry if not specified
	strategyName := "prefix-source-registry"
	log.Debug("Using default path strategy", "strategy", strategyName)

	// Initialize and return the strategy
	pathStrategy, err := strategy.GetStrategy(strategyName, config.Mappings)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize path strategy: %w", err)
	}
	return pathStrategy, nil
}

// loadRegistryMappings loads registry mappings from the specified file.
func loadRegistryMappings(cmd *cobra.Command, config *GeneratorConfig) error {
	// Nil check for safety
	if config == nil {
		return errors.New("loadRegistryMappings: config parameter is nil")
	}

	// Prioritize the registry-file flag, fallback to the deprecated config flag
	registryFilePath, registryErr := cmd.Flags().GetString("registry-file")
	if registryErr != nil {
		return fmt.Errorf("failed to get registry-file flag: %w", registryErr)
	}

	deprecatedConfigPath, configErr := cmd.Flags().GetString("config")
	if configErr != nil {
		return fmt.Errorf("failed to get config flag: %w", configErr)
	}

	configFileName := registryFilePath
	if configFileName == "" {
		// Try deprecated flag
		configFileName = deprecatedConfigPath
		if configFileName == "" {
			log.Debug("No registry mapping file specified")
			// This is not an error condition, just a configuration choice
			return nil
		}
		log.Warn("Using deprecated --config flag, please use --registry-file instead")
	}

	// Get current working directory - use the global isTestMode variable
	skipCWDRestriction := integrationTestMode || (os.Getenv("IRR_TESTING") == trueString)

	// Load mappings file
	mappingsConfig, err := registry.LoadConfigDefault(configFileName, skipCWDRestriction)
	if err != nil {
		return fmt.Errorf("failed to load registry mappings from file %s: %w", configFileName, err)
	}

	// Convert structured Config to the simpler Mappings
	config.Mappings = mappingsConfig.ToMappings()

	if config.Mappings != nil {
		log.Info("Registry mappings loaded successfully", "count", len(config.Mappings.Entries))

		// Derive source registries from mappings if not explicitly provided
		deriveSourceRegistriesFromMappings(config)
	} else {
		log.Info("No registry mappings loaded from file", "file", configFileName)
	}

	return nil
}

// validateUnmappableRegistries checks if all provided source registries are covered by mappings.
// It logs warnings or returns an error based on strict mode.
func validateUnmappableRegistries(config *GeneratorConfig) error {
	// Add nil check for safety
	if config == nil {
		return errors.New("internal error: validateUnmappableRegistries called with nil config")
	}

	if len(config.SourceRegistries) == 0 {
		// No source registries provided, nothing to validate
		return nil
	}

	// Check if mappings exist
	hasMappings := (config.Mappings != nil && len(config.Mappings.Entries) > 0)

	// If NO mappings exist at all, check all source registries.
	if !hasMappings {
		if config.StrictMode {
			// Strict mode requires mappings if source registries are specified
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitRegistryDetectionError,
				Err:  fmt.Errorf("strict mode enabled: no mapping found for registries: %s", strings.Join(config.SourceRegistries, ", ")),
			}
		}
		// Non-strict mode: Log warning about all source registries needing mapping
		log.Warn("No mapping found for registries", "registries", strings.Join(config.SourceRegistries, ", "))
		log.Info("These registries will be redirected using the target registry", "target", config.TargetRegistry)
		log.Info("To add mappings, use: irr config --source <registry> --target <path>")
		for _, reg := range config.SourceRegistries {
			log.Info("irr config suggestion", "source", reg, "target", fmt.Sprintf("%s/%s", config.TargetRegistry, strings.ReplaceAll(reg, ".", "-")))
		}
		return nil // Don't error in non-strict mode
	}

	// If mappings *do* exist, check each source registry individually
	unmappableRegistries := make([]string, 0)
	for _, sourceReg := range config.SourceRegistries {
		found := false
		if config.Mappings != nil {
			for _, mapping := range config.Mappings.Entries {
				if mapping.Source == sourceReg {
					found = true
					break
				}
			}
		}
		if !found {
			if strings.HasPrefix(config.TargetRegistry, sourceReg) {
				found = true
			}
		}
		if !found {
			unmappableRegistries = append(unmappableRegistries, sourceReg)
		}
	}
	if len(unmappableRegistries) > 0 {
		if config.StrictMode {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitRegistryDetectionError,
				Err:  fmt.Errorf("strict mode enabled: no mapping found for registries: %s", strings.Join(unmappableRegistries, ", ")),
			}
		}
		log.Warn("No mapping found for registries", "registries", strings.Join(unmappableRegistries, ", "))
		log.Info("These registries will be redirected using the target registry", "target", config.TargetRegistry)
		log.Info("To add mappings, use: irr config --source <registry> --target <path>")
		for _, reg := range unmappableRegistries {
			log.Info("irr config suggestion", "source", reg, "target", fmt.Sprintf("%s/%s", config.TargetRegistry, strings.ReplaceAll(reg, ".", "-")))
		}
	}
	return nil
}

// Helper to perform context-aware chart analysis (deduplicates logic)
func performContextAwareAnalysis(chartPath string, valueOpts *values.Options) (*helmchart.Chart, *analysis.ChartAnalysis, error) {
	// Add nil check for valueOpts, although the call site should prevent this
	if valueOpts == nil {
		log.Error("Internal error: performContextAwareAnalysis called with nil valueOpts")
		// Return an internal error, as this indicates a programming mistake in the caller
		return nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInternalError,
			Err:  errors.New("internal error: valueOpts cannot be nil in performContextAwareAnalysis"),
		}
	}
	loaderOptions := &internalhelm.ChartLoaderOptions{
		ChartPath:  chartPath,
		ValuesOpts: *valueOpts, // Dereference is now safe
	}
	chartLoader := internalhelm.NewChartLoader()
	chartAnalysisContext, loadErr := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
	switch {
	case loadErr != nil:
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("failed to load chart with values: %w", loadErr)}
	case chartAnalysisContext == nil:
		return nil, nil, errors.New("internal error: LoadChartAndTrackOrigins returned nil context without error")
	case chartAnalysisContext.Chart == nil:
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: errors.New("failed to load chart details from context")}
	}
	contextAnalyzer := internalhelm.NewContextAwareAnalyzer(chartAnalysisContext)
	chartAnalysis, analyzeErr := contextAnalyzer.AnalyzeContext()
	if analyzeErr != nil {
		return nil, nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("context analysis failed: %w", analyzeErr)}
	}
	return chartAnalysisContext.Chart, chartAnalysis, nil
}

// createAndExecuteGenerator creates and executes a generator for the given chart source
func createAndExecuteGenerator(cmd *cobra.Command, config *GeneratorConfig, contextAware bool) ([]byte, error) {
	log.Info("Initializing override generation", "chartPath", config.ChartPath)

	var loadedChart *helmchart.Chart
	var analysisResult *analysis.ChartAnalysis
	var loadAnalysisErr error

	valueOpts, err := getValuesOptionsFromFlags(cmd)
	if err != nil {
		return nil, err
	}

	if contextAware {
		log.Info("Performing context-aware chart analysis...")
		loadedChart, analysisResult, loadAnalysisErr = performContextAwareAnalysis(config.ChartPath, &valueOpts)
	} else {
		log.Info("Performing legacy chart analysis...")
		legacyLoader := chart.NewLoader()
		var loadErr error
		var legacyLoadedChart *helmchart.Chart
		legacyLoadedChart, loadErr = legacyLoader.Load(config.ChartPath)
		if loadErr != nil {
			loadAnalysisErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("legacy chart load failed: %w", loadErr)}
		} else {
			loadedChart = legacyLoadedChart
			analyzer := analysis.NewAnalyzer(config.ChartPath, legacyLoader)
			var legacyAnalysisResult *analysis.ChartAnalysis
			legacyAnalysisResult, loadErr = analyzer.Analyze()
			if loadErr != nil {
				loadAnalysisErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("legacy analysis failed: %w", loadErr)}
			} else {
				analysisResult = legacyAnalysisResult
			}
		}
	}

	if loadAnalysisErr != nil {
		log.Error("Chart loading/analysis failed", "error", loadAnalysisErr)
		return nil, loadAnalysisErr
	}
	if loadedChart == nil {
		log.Error("Internal error: loadedChart is nil after load/analysis phase without error")
		return nil, &exitcodes.ExitCodeError{Code: exitcodes.ExitGeneralRuntimeError, Err: errors.New("internal error: loadedChart missing")}
	}
	if analysisResult == nil {
		log.Warn("Analysis result is nil (e.g., chart has no values/images), proceeding with empty analysis.")
		analysisResult = analysis.NewChartAnalysis()
	}

	pathStrategy, err := setupPathStrategy(config)
	if err != nil {
		return nil, fmt.Errorf("failed to set up path strategy: %w", err)
	}
	config.Strategy = pathStrategy

	generator, err := createGenerator(config, contextAware)
	if err != nil {
		return nil, err
	}

	// Add nil check for config before accessing its fields for logging
	logChartPath := nilConfigPlaceholder
	logTargetReg := nilConfigPlaceholder
	logStrategyType := nilConfigPlaceholder
	logStrategyIsNil := true
	logConfigPtr := nilConfigPlaceholder
	if config != nil {
		logChartPath = config.ChartPath
		logTargetReg = config.TargetRegistry
		logStrategyType = fmt.Sprintf("%T", config.Strategy)
		logStrategyIsNil = config.Strategy == nil
		logConfigPtr = fmt.Sprintf("%p", config)
	}

	log.Debug("Creating generator instance just before NewGenerator call",
		"chartPath", logChartPath,
		"targetRegistry", logTargetReg,
		"strategy_type", logStrategyType,
		"strategy_is_nil", logStrategyIsNil,
		"config_ptr", logConfigPtr)

	overrideResult, err := generator.Generate(loadedChart, analysisResult)
	if err != nil {
		return nil, handleGenerateError(err)
	}

	yamlBytes, err := yaml.Marshal(overrideResult.Values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal overrides to YAML: %w", err)
	}

	return yamlBytes, nil
}

// createGenerator creates a generator based on the context-aware flag.
func createGenerator(config *GeneratorConfig, contextAware bool) (*chart.Generator, error) {
	if config == nil {
		return nil, errors.New("nil generator config")
	}

	// Ensure strategy is initialized
	if config.Strategy == nil {
		var err error
		config.Strategy, err = strategy.GetStrategy("prefix-source-registry", config.Mappings)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize default strategy: %w", err)
		}
		log.Debug("Strategy was nil, set default", "strategy", config.Strategy)
	}

	var preloadedLoader *PreloadedChartLoader
	var generatorErr error

	if contextAware {
		log.Info("Creating generator using context-aware analysis...")
		// --- Context-Aware Path ---
		loaderOptions := &internalhelm.ChartLoaderOptions{
			ChartPath: config.ChartPath,
			// No other options needed for initial load in standalone mode
		}
		chartLoader := internalhelm.NewChartLoader()
		chartAnalysisContext, loadErr := chartLoader.LoadChartAndTrackOrigins(loaderOptions)
		switch {
		case loadErr != nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("context-aware chart load failed: %w", loadErr)}
		case chartAnalysisContext == nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitInternalError, Err: errors.New("internal error: nil chart context without error")}
		case chartAnalysisContext.Chart == nil:
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: errors.New("loaded chart context contains nil chart")}
		default:
			// Chart is loaded, create analyzer
			contextAnalyzer := internalhelm.NewContextAwareAnalyzer(chartAnalysisContext)
			chartAnalysis, analyzeErr := contextAnalyzer.AnalyzeContext()
			if analyzeErr != nil {
				generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("context analysis failed: %w", analyzeErr)}
			} else {
				// Analysis completed, prepare preloader
				preloadedLoader = &PreloadedChartLoader{
					chart:    chartAnalysisContext.Chart,
					analysis: chartAnalysis,
				}
			}
		}
	} else {
		log.Info("Creating generator using legacy analysis...")
		// --- Legacy Path ---
		// Use the standard chart loader from pkg/chart
		legacyLoader := chart.NewLoader() // Assuming NewLoader exists in pkg/chart
		var loadedChart *helmchart.Chart
		var analysisResult *analysis.ChartAnalysis
		var loadErr error // Declare loadErr for this block scope
		loadedChart, loadErr = legacyLoader.Load(config.ChartPath)
		if loadErr != nil {
			generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartLoadFailed, Err: fmt.Errorf("legacy chart load failed: %w", loadErr)}
		} else {
			analyzer := analysis.NewAnalyzer(config.ChartPath, legacyLoader)
			analysisResult, loadErr = analyzer.Analyze()
			if loadErr != nil {
				generatorErr = &exitcodes.ExitCodeError{Code: exitcodes.ExitChartProcessingFailed, Err: fmt.Errorf("legacy analysis failed: %w", loadErr)}
			} else {
				// Setup preloaded loader on success
				preloadedLoader = &PreloadedChartLoader{
					chart:    loadedChart,
					analysis: analysisResult,
				}
			}
		}
	}

	if generatorErr != nil {
		return nil, generatorErr
	}

	if preloadedLoader == nil {
		return nil, errors.New("internal error: failed to prepare chart analysis data for generator")
	}

	// Add log before calling NewGenerator
	log.Debug("Creating generator instance just before NewGenerator call",
		"chartPath", config.ChartPath,
		"targetRegistry", config.TargetRegistry,
		"strategy_type", fmt.Sprintf("%T", config.Strategy),
		"strategy_is_nil", config.Strategy == nil,
		"config_ptr", fmt.Sprintf("%p", config))

	// --- Create Override Generator (Common logic) ---
	generator := chart.NewGenerator(
		config.ChartPath,
		config.TargetRegistry,
		config.SourceRegistries,
		config.ExcludeRegistries,
		config.Strategy,
		config.Mappings,
		config.StrictMode,
		0,
		preloadedLoader,
		config.RulesEnabled,
	)

	// Log message if rules are disabled
	if !config.RulesEnabled {
		log.Info("Chart parameter rules system is disabled")
	}

	return generator, nil
}

// PreloadedChartLoader is a custom loader that returns a pre-loaded chart and analysis.
// It implements the chart.Loader interface.
type PreloadedChartLoader struct {
	chart    *helmchart.Chart
	analysis *analysis.ChartAnalysis
}

// Load implements the chart.Loader interface.
func (l *PreloadedChartLoader) Load(_ string) (*helmchart.Chart, error) {
	return l.chart, nil
}

// Analyze implements the analysis.ChartLoader interface.
func (l *PreloadedChartLoader) Analyze(_ string) (*analysis.ChartAnalysis, error) {
	return l.analysis, nil
}

// runOverrideStandaloneMode handles override generation when running in standalone mode.
func runOverrideStandaloneMode(cmd *cobra.Command, outputFile string, dryRun, isPluginOperatingOnRelease bool) error {
	generatorConfig, err := setupGeneratorConfig(cmd, isPluginOperatingOnRelease)
	if err != nil {
		return err
	}

	// Load registry mappings after setting up the basic config
	if err := loadRegistryMappings(cmd, &generatorConfig); err != nil {
		return err
	}

	if generatorConfig.Mappings != nil {
		log.Info("Registry mappings loaded successfully", "count", len(generatorConfig.Mappings.Entries))
	} else {
		log.Info("No registry mapping file provided or mappings are empty.")
	}

	// Derive source registries from mappings if not explicitly provided.
	deriveSourceRegistriesFromMappings(&generatorConfig)

	// Setup Path Strategy (must be after mappings are loaded and sources derived)
	pathStrategy, err := setupPathStrategy(&generatorConfig)
	if err != nil {
		return err
	}
	generatorConfig.Strategy = pathStrategy

	contextAware, err := getBoolFlag(cmd, "context-aware")
	if err != nil {
		return err
	}
	yamlBytes, err := createAndExecuteGenerator(cmd, &generatorConfig, contextAware)
	if err != nil {
		return err
	}
	return outputOverrides(cmd, yamlBytes, outputFile, dryRun)
}
