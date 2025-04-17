package helm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/exitcodes"
	"github.com/lalbers/irr/pkg/fileutil"
	"github.com/lalbers/irr/pkg/image"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
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
	// Replace dots and slashes with nothing
	s := registry
	s = filepath.Base(s) // Remove any paths
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// ValidateRelease validates a Helm release with override values
func (a *Adapter) ValidateRelease(ctx context.Context, releaseName, namespace string, overrideFiles []string) error {
	// Validate plugin mode
	if !a.isRunningAsPlugin {
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitInputConfigurationError,
			Err:  fmt.Errorf("the release name flag is only available when running as a Helm plugin (helm irr ...)"),
		}
	}

	// Add nil check for the Helm client
	if a.helmClient == nil {
		log.Errorf("Helm client is nil in ValidateRelease, this is likely a configuration error")
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("helm client is nil (configuration error)"),
		}
	}

	// Add nil check for the filesystem
	if a.fs == nil {
		log.Errorf("Filesystem is nil in ValidateRelease, this is likely a configuration error")
		return &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  fmt.Errorf("filesystem is nil (configuration error)"),
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

	// Resolve chart path required for template rendering
	chartPath, err := a.resolveChartPath(chartMeta)
	if err != nil {
		return fmt.Errorf("could not resolve chart path: %w", err)
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
		// TODO: Implement values merging properly
		// For now, just shallow merge the keys
		for k, v := range overrideValue {
			values[k] = v
		}
	}

	// Perform validation
	// For now, just attempt to template with the override values
	manifest, err := a.helmClient.TemplateChart(ctx, releaseName, chartPath, values, namespace)
	if err != nil {
		return fmt.Errorf("template rendering failed: %w", err)
	}

	if manifest == "" {
		return fmt.Errorf("template rendered empty manifest, this may indicate an issue")
	}

	return nil
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

	// Prepare temp directory for chart files
	tempDir := os.TempDir()
	tempChartPath := filepath.Join(tempDir, "irr", "charts", meta.Name+"-"+meta.Version)

	// Check if this chart has already been cached
	_, err := a.fs.Stat(tempChartPath)
	if err == nil {
		debug.Printf("Using cached chart at %s", tempChartPath)
		return tempChartPath, nil
	}

	// Create the directory structure
	if err := a.fs.MkdirAll(tempChartPath, DirMode); err != nil {
		return "", fmt.Errorf("failed to create temporary chart directory: %w", err)
	}

	// At this point, we couldn't find the chart, return the temp path as a placeholder
	return tempChartPath, nil
}
