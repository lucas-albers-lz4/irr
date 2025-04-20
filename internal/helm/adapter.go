package helm

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
)

// FileMode constants for directories and files
const (
	DirMode = 0o755 // standard executable directory
)

// Adapter bridges between Helm plugin functionality and core IRR functions
type Adapter struct {
	helmClient        ClientInterface
	fs                afero.Fs
	isRunningAsPlugin bool
}

// AnalysisResult represents the result of chart analysis
type AnalysisResult struct {
	ChartInfo chart.Info                 `json:"chart" yaml:"chart"`
	Images    map[string]image.Reference `json:"images" yaml:"images"`
}

// ToYAML converts analysis result to YAML
func (r *AnalysisResult) ToYAML() (string, error) {
	bytes, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal analysis result to YAML: %w", err)
	}
	return string(bytes), nil
}

// OverrideOptions represents options for generating overrides
type OverrideOptions struct {
	TargetRegistry   string
	SourceRegistries []string
	StrictMode       bool
	PathStrategy     string
}

// NewAdapter creates a new Helm adapter
func NewAdapter(helmClient ClientInterface, fs afero.Fs, isPlugin bool) *Adapter {
	return &Adapter{
		helmClient:        helmClient,
		fs:                fs,
		isRunningAsPlugin: isPlugin,
	}
}

// InspectRelease inspects a Helm release to identify image references
func (a *Adapter) InspectRelease(ctx context.Context, releaseName, namespace, outputFile string) error {
	// Validate plugin mode
	if !a.isRunningAsPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
		}
	}

	// Get release values from Helm
	values, err := a.helmClient.GetReleaseValues(ctx, releaseName, namespace)
	if err != nil {
		if IsReleaseNotFoundError(err) {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err: fmt.Errorf("release %q not found in namespace %q, verify that the release exists with: helm list -n %s",
					releaseName, namespace, namespace),
			}
		}
		return fmt.Errorf("failed to get values for release %q: %w", releaseName, err)
	}

	// Get chart metadata for the release
	chartMeta, err := a.helmClient.GetReleaseChart(ctx, releaseName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get chart metadata for release %q: %w", releaseName, err)
	}

	// Resolve chart path or use temporary path as fallback
	chartPath, err := a.resolveChartPath(chartMeta)
	if err != nil {
		log.Warnf("Could not resolve chart path for %s:%s, using best effort approach", chartMeta.Name, chartMeta.Version)
		// Continue even if we couldn't resolve the chart path - we can still analyze values
	}

	// Create detector for image analysis
	// Fix: Use proper constructor to initialize the detector with a valid context
	detectionContext := image.DetectionContext{
		SourceRegistries:  []string{}, // Empty for inspection
		ExcludeRegistries: []string{},
	}

	// Initialize detector properly
	detector := image.NewDetector(detectionContext)

	// Detect images in values
	detectedImages, unsupported, err := detector.DetectImages(values, nil)
	if err != nil {
		return fmt.Errorf("failed to detect images in release values: %w", err)
	}

	// Convert detected images to map
	imageMap := make(map[string]image.Reference)
	for _, img := range detectedImages {
		if img.Reference != nil {
			imageMap[img.Path[0]] = *img.Reference
		}
	}

	if len(unsupported) > 0 {
		log.Warnf("Found %d unsupported image structures", len(unsupported))
	}

	// Create result
	result := &AnalysisResult{
		ChartInfo: chart.Info{
			Name:    chartMeta.Name,
			Version: chartMeta.Version,
			Path:    chartPath,
		},
		Images: imageMap,
	}

	// Convert to YAML
	output, err := result.ToYAML()
	if err != nil {
		return fmt.Errorf("failed to convert analysis result to YAML: %w", err)
	}

	// Output to file or stdout
	if outputFile != "" {
		// Check if file exists
		if _, err := a.fs.Stat(outputFile); err == nil {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitInputConfigurationError,
				Err:  fmt.Errorf("output file %q already exists", outputFile),
			}
		}

		// Write to file
		err = afero.WriteFile(a.fs, outputFile, []byte(output), fileutil.ReadWriteUserPermission)
		if err != nil {
			return fmt.Errorf("failed to write output to file %q: %w", outputFile, err)
		}
		log.Infof("Analysis result written to %s", outputFile)
	} else {
		// Write to stdout
		fmt.Println(output)
	}

	return nil
}

// OverrideRelease generates Helm value overrides for a release to redirect images
func (a *Adapter) OverrideRelease(ctx context.Context, releaseName, namespace string, targetRegistry string,
	sourceRegistries []string, pathStrategy string, options OverrideOptions) (string, error) {
	// Validate plugin mode
	if !a.isRunningAsPlugin {
		return "", &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
		}
	}

	// Get release values from Helm
	values, err := a.helmClient.GetReleaseValues(ctx, releaseName, namespace)
	if err != nil {
		if IsReleaseNotFoundError(err) {
			return "", &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err: fmt.Errorf("release %q not found in namespace %q, verify that the release exists with: helm list -n %s",
					releaseName, namespace, namespace),
			}
		}
		return "", fmt.Errorf("failed to get values for release %q: %w", releaseName, err)
	}

	// Get chart metadata for the release (logging purposes only)
	_, err = a.helmClient.GetReleaseChart(ctx, releaseName, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get chart metadata for release %q: %w", releaseName, err)
	}

	// Set up options as best we can
	// Fix: Use proper constructor to initialize the detector with a valid context
	// Create a detection context with source registries from the function parameter
	detectionContext := image.DetectionContext{
		SourceRegistries:  sourceRegistries, // Use the source registries from the parameter
		ExcludeRegistries: []string{},
		GlobalRegistry:    targetRegistry,     // Set the global registry from the parameter
		Strict:            options.StrictMode, // Use the strict mode option
	}

	// Initialize detector properly
	detector := image.NewDetector(detectionContext)
	detectedImages, _, err := detector.DetectImages(values, nil)
	if err != nil {
		return "", fmt.Errorf("failed to detect images in release values: %w", err)
	}

	// Generate overrides map manually
	overrideMap := make(map[string]interface{})

	for _, img := range detectedImages {
		if img.Reference == nil {
			continue
		}

		// Implement simple prefix-based path strategy
		imgRef := *img.Reference
		newRepo := imgRef.Repository
		if pathStrategy == "prefix-source-registry" {
			// Sanitize registry for path
			registrySanitized := sanitizeRegistryForPath(imgRef.Registry)
			newRepo = registrySanitized + "/" + imgRef.Repository
		}

		// Create override structure at the correct path
		path := img.Path
		current := overrideMap

		// Build nested structure
		for i := 0; i < len(path)-1; i++ {
			key := path[i]
			if _, exists := current[key]; !exists {
				current[key] = make(map[string]interface{})
			}
			if nextLevel, ok := current[key].(map[string]interface{}); ok {
				current = nextLevel
			} else {
				log.Warnf("Unexpected type for key %s, expected map", key)
				break
			}
		}

		// Set the final value
		lastKey := path[len(path)-1]
		current[lastKey] = map[string]interface{}{
			"registry":   targetRegistry,
			"repository": newRepo,
		}

		// Add tag or digest
		if imgRef.Digest != "" {
			if valueMap, ok := current[lastKey].(map[string]interface{}); ok {
				valueMap["digest"] = imgRef.Digest
			}
		} else if imgRef.Tag != "" {
			if valueMap, ok := current[lastKey].(map[string]interface{}); ok {
				valueMap["tag"] = imgRef.Tag
			}
		}
	}

	// Convert to YAML
	yamlBytes, err := yaml.Marshal(overrideMap)
	if err != nil {
		return "", fmt.Errorf("failed to convert overrides to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// sanitizeRegistryForPath sanitizes a registry name for use in a path
func sanitizeRegistryForPath(registry string) string {
	// Split off port if present
	hostPart := registry
	if host, _, err := net.SplitHostPort(registry); err == nil {
		hostPart = host
	}

	// Replace dots and dashes with nothing
	s := hostPart
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// ValidateRelease validates a Helm release with override values
func (a *Adapter) ValidateRelease(ctx context.Context, releaseName, namespace string, overrideFiles []string, kubeVersion string) error {
	// Validate inputs
	if a.helmClient == nil {
		log.Errorf("Helm client is nil in ValidateRelease, this is likely a configuration error")
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("helm client not properly initialized"),
		}
	}

	if a.fs == nil {
		log.Errorf("Filesystem is nil in ValidateRelease, this is likely a configuration error")
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("filesystem not properly initialized"),
		}
	}

	debug.Printf("ValidateRelease called with kubeVersion=%q", kubeVersion)

	// Validate plugin mode
	if !a.isRunningAsPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
		}
	}

	// Get release values from Helm
	values, err := a.helmClient.GetReleaseValues(ctx, releaseName, namespace)
	if err != nil {
		if IsReleaseNotFoundError(err) {
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err: fmt.Errorf("release %q not found in namespace %q, verify that the release exists with: helm list -n %s",
					releaseName, namespace, namespace),
			}
		}
		return fmt.Errorf("failed to get values for release %q: %w", releaseName, err)
	}

	// Get chart metadata for the release
	chartMeta, err := a.helmClient.GetReleaseChart(ctx, releaseName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get chart metadata for release %q: %w", releaseName, err)
	}

	// Add nil check for chartMeta
	if chartMeta == nil {
		log.Errorf("Chart metadata is nil for release %q in namespace %q", releaseName, namespace)
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitChartNotFound,
			Err:  fmt.Errorf("chart metadata is nil for release %q", releaseName),
		}
	}

	// Use improved chart resolution with better fallback handling
	var chartPath string
	// Check if we have access to FindChartForRelease method (available if client is RealHelmClient)
	if realClient, ok := a.helmClient.(*RealHelmClient); ok {
		chartPath, err = realClient.FindChartForRelease(ctx, releaseName, namespace)
		if err != nil {
			log.Warnf("Failed to find chart for release using advanced lookup: %v", err)
			log.Infof("Falling back to basic chart resolution method")
			// Fall back to resolveChartPath if advanced method fails
			chartPath, err = a.resolveChartPath(chartMeta)
			if err != nil {
				return fmt.Errorf("could not resolve chart path: %w", err)
			}
		} else {
			log.Infof("Found chart for release at: %s", chartPath)
		}
	} else {
		// Use the regular chart resolution if we can't access the advanced method
		chartPath, err = a.resolveChartPath(chartMeta)
		if err != nil {
			return fmt.Errorf("could not resolve chart path: %w", err)
		}
	}

	// Load values files
	var overrideValues []map[string]interface{}
	for _, file := range overrideFiles {
		valuesMap, err := loadValuesFile(a.fs, file)
		if err != nil {
			return fmt.Errorf("failed to load values file %q: %w", file, err)
		}
		overrideValues = append(overrideValues, valuesMap)
	}

	// Merge all values
	for _, overrideValue := range overrideValues {
		// Shallow merge the keys for now
		for k, v := range overrideValue {
			values[k] = v
		}
	}

	// Perform validation
	// For now, just attempt to template with the override values
	_, err = a.helmClient.TemplateChart(ctx, releaseName, chartPath, values, namespace, kubeVersion)
	if err != nil {
		// If templating fails with a "Chart.yaml file is missing" error, try to handle it
		if strings.Contains(err.Error(), "Chart.yaml file is missing") {
			log.Warnf("Chart.yaml file is missing for %s", chartPath)

			// Try to find another path
			if realClient, ok := a.helmClient.(*RealHelmClient); ok {
				// Try even harder to find the chart, with more aggressive search
				altPath, altErr := handleChartYamlMissingWithSDK(releaseName, "", chartPath, realClient)
				if altErr == nil && altPath != "" {
					log.Infof("Found alternative chart path: %s", altPath)

					// Try templating again with the new path
					_, err = a.helmClient.TemplateChart(ctx, releaseName, altPath, values, namespace, kubeVersion)
					if err == nil {
						// Success with alternative path!
						log.Infof("Successfully validated chart with alternative path")
						return nil
					}
					log.Warnf("Failed to validate even with alternative path: %v", err)
				}
			}

			// If we couldn't resolve it, return a more helpful error
			return &exitcodes.ExitCodeError{
				Code: exitcodes.ExitChartNotFound,
				Err: fmt.Errorf("chart.yaml file is missing for release %q. Try providing the chart path directly with --chart-path flag",
					releaseName),
			}
		}

		return fmt.Errorf("template rendering failed: %w", err)
	}

	if err == nil {
		return nil
	}

	return fmt.Errorf("template rendered empty manifest, this may indicate an issue")
}

// handleChartYamlMissingWithSDK is a helper function to find charts using Helm SDK
// when the initial chart path fails with "Chart.yaml file is missing"
func handleChartYamlMissingWithSDK(_, _, originalChartPath string, _ *RealHelmClient) (string, error) {
	// Extract chart name from the path
	chartName := filepath.Base(originalChartPath)
	chartName = strings.TrimSuffix(chartName, ".tgz")

	// Extract potential version from name-version pattern
	var chartVersion string
	parts := strings.Split(chartName, "-")
	if len(parts) > 1 {
		// Try to detect if last part is a version number
		lastPart := parts[len(parts)-1]
		if lastPart != "" && lastPart[0] >= '0' && lastPart[0] <= '9' {
			chartVersion = lastPart
			chartName = strings.Join(parts[:len(parts)-1], "-")
		}
	}

	debug.Printf("Extracted chart name: %s, version: %s from path: %s", chartName, chartVersion, originalChartPath)

	// Try to use LocateChart directly
	settings := cli.New()
	chartPathOptions := action.ChartPathOptions{
		Version: chartVersion,
	}

	// Try with just the chart name
	chartPath, err := chartPathOptions.LocateChart(chartName, settings)
	if err == nil {
		debug.Printf("Successfully located chart at %s using chart name only", chartPath)
		return chartPath, nil
	}

	// Try repository cache locations directly
	cacheDir := settings.RepositoryCache
	if cacheDir != "" {
		debug.Printf("Checking repository cache at %s", cacheDir)

		// Try with exact version if available
		if chartVersion != "" {
			cachePath := filepath.Join(cacheDir, fmt.Sprintf("%s-%s.tgz", chartName, chartVersion))
			if _, err := os.Stat(cachePath); err == nil {
				debug.Printf("Found chart at %s", cachePath)
				return cachePath, nil
			}
		}

		// Try with glob pattern
		pattern := filepath.Join(cacheDir, fmt.Sprintf("%s-*.tgz", chartName))
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			// Sort to get latest version
			sort.Strings(matches)
			chartPath := matches[len(matches)-1]
			debug.Printf("Found chart using glob at %s", chartPath)
			return chartPath, nil
		}
	}

	// Failed to find chart
	return "", fmt.Errorf("could not locate chart for %s even with fallbacks", chartName)
}

// loadValuesFile loads YAML values from a file
func loadValuesFile(fs afero.Fs, filename string) (map[string]interface{}, error) {
	// Read the file
	data, err := afero.ReadFile(fs, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read values file %q: %w", filename, err)
	}

	// Parse YAML
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("failed to parse values file %q: %w", filename, err)
	}

	return values, nil
}

// resolveChartPath attempts to resolve the filesystem path for a chart based on metadata
func (a *Adapter) resolveChartPath(meta *ChartMetadata) (string, error) {
	// If the chart metadata already has a path (rare), use it
	if meta.Path != "" {
		return meta.Path, nil
	}

	// Create Helm settings to access Helm's configuration
	settings := cli.New()

	// Create a chart path options object to leverage Helm's chart location functionality
	chartPathOptions := action.ChartPathOptions{
		Version: meta.Version,
		RepoURL: meta.Repository,
	}

	// Try to locate the chart using Helm's built-in functionality
	// This will check Helm's cache and repositories
	chartRef := meta.Name
	if meta.Repository != "" {
		// If we have repository information, use it
		chartRef = fmt.Sprintf("%s/%s", meta.Repository, meta.Name)
	}

	debug.Printf("Attempting to locate chart %s version %s using Helm SDK", chartRef, meta.Version)
	chartPath, err := chartPathOptions.LocateChart(chartRef, settings)
	if err == nil {
		debug.Printf("Found chart using Helm SDK at: %s", chartPath)
		return chartPath, nil
	}
	debug.Printf("Failed to locate chart using Helm SDK: %v", err)

	// If Helm SDK couldn't find it, try searching in Helm's repository cache directly
	cacheDir := settings.RepositoryCache
	if cacheDir != "" {
		debug.Printf("Checking Helm repository cache at: %s", cacheDir)
		chartTgz := fmt.Sprintf("%s-%s.tgz", meta.Name, meta.Version)
		cachePath := filepath.Join(cacheDir, chartTgz)

		exists, err := afero.Exists(a.fs, cachePath)
		if err == nil && exists {
			debug.Printf("Found chart in Helm cache: %s", cachePath)
			return cachePath, nil
		}

		// Try glob pattern if exact match failed
		matches, err := afero.Glob(a.fs, filepath.Join(cacheDir, meta.Name+"-*.tgz"))
		if err == nil && len(matches) > 0 {
			// Sort to get the latest version if multiple exist
			sort.Strings(matches)
			chartPath := matches[len(matches)-1] // Get the last (likely highest version)
			debug.Printf("Found chart in Helm cache (glob match): %s", chartPath)
			return chartPath, nil
		}
	}

	// Fall back to checking common Helm cache directories
	helmCachePaths := []string{
		// macOS Helm cache path
		filepath.Join(os.Getenv("HOME"), "Library", "Caches", "helm", "repository"),
		// Linux/Unix Helm cache path
		filepath.Join(os.Getenv("HOME"), ".cache", "helm", "repository"),
		// Windows Helm cache path - uses APPDATA
		filepath.Join(os.Getenv("APPDATA"), "helm", "repository"),
	}

	for _, cachePath := range helmCachePaths {
		// Skip if this is the same as the repository cache we already checked
		if cachePath == cacheDir {
			continue
		}

		// Look for exact match with name-version.tgz
		potentialChartPath := filepath.Join(cachePath, meta.Name+"-"+meta.Version+".tgz")
		debug.Printf("Checking for chart in cache path: %s", potentialChartPath)

		exists, err := afero.Exists(a.fs, potentialChartPath)
		if err == nil && exists {
			debug.Printf("Found chart in cache: %s", potentialChartPath)
			return potentialChartPath, nil
		}

		// Also try to glob search for any matching version
		matches, err := afero.Glob(a.fs, filepath.Join(cachePath, meta.Name+"-*.tgz"))
		if err == nil && len(matches) > 0 {
			// Sort to get the latest version if multiple exist
			sort.Strings(matches)
			chartPath := matches[len(matches)-1] // Get the last (likely highest version)
			debug.Printf("Found chart in cache (glob match): %s", chartPath)
			return chartPath, nil
		}
	}

	// Prepare temp directory for chart files if not found in cache
	tempDir := os.TempDir()
	tempChartPath := filepath.Join(tempDir, "irr", "charts", meta.Name+"-"+meta.Version)

	// Check if this chart has already been cached in our temp dir
	_, err = a.fs.Stat(tempChartPath)
	if err == nil {
		debug.Printf("Using cached chart at %s", tempChartPath)
		return tempChartPath, nil
	}

	// Create the directory structure
	if err := a.fs.MkdirAll(tempChartPath, DirMode); err != nil {
		return "", fmt.Errorf("failed to create temporary chart directory: %w", err)
	}

	// At this point, we couldn't find the chart, return the temp path as a placeholder
	debug.Printf("Could not find chart in any cache, using temporary path: %s", tempChartPath)
	return tempChartPath, nil
}

// Add wrapper methods to expose client functionality

// GetReleaseValues retrieves the computed values for a deployed release.
func (a *Adapter) GetReleaseValues(ctx context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	return a.helmClient.GetReleaseValues(ctx, releaseName, namespace)
}

// GetChartFromRelease retrieves the chart metadata associated with a deployed release.
func (a *Adapter) GetChartFromRelease(ctx context.Context, releaseName, namespace string) (*ChartMetadata, error) {
	return a.helmClient.GetReleaseChart(ctx, releaseName, namespace)
}
