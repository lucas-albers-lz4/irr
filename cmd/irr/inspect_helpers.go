// Package main contains the implementation for the irr CLI, including subcommands like inspect.
package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli/values"

	"github.com/lucas-albers-lz4/irr/internal/helm"
	"github.com/lucas-albers-lz4/irr/pkg/analysis"
	"github.com/lucas-albers-lz4/irr/pkg/exitcodes"
	"github.com/lucas-albers-lz4/irr/pkg/image"
	log "github.com/lucas-albers-lz4/irr/pkg/log"
)

// createHelmClient creates a new instance of the Helm client
func createHelmClient() (helm.ClientInterface, error) {
	client, err := helm.NewHelmClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Helm client: %w", err)
	}
	return client, nil
}

// detectChartIfNeeded determines the chart path if not provided.
// It prioritizes the provided chart path. If empty, it calls detectChartInCurrentDirectory.
func detectChartIfNeeded(fs afero.Fs, inputChartPath string) (finalAbsPath, finalRelPath string, err error) {
	log.Debug("detectChartIfNeeded: Start", "inputChartPath", inputChartPath)
	if inputChartPath != "" {
		log.Debug("detectChartIfNeeded: Chart path provided, skipping detection", "chartPath", inputChartPath)
		// Return the input path and "." for relative path as detection was skipped.
		return inputChartPath, ".", nil
	}

	log.Debug("detectChartIfNeeded: No chart path provided, searching current directory.")
	detectedPath, relativePath, err := detectChartInCurrentDirectory(fs)
	if err != nil {
		// Wrap the error from detection.
		return "", "", fmt.Errorf("chart path not specified and error occurred during detection: %w", err)
	}
	log.Debug("detectChartIfNeeded: Detected chart path", "detectedPath", detectedPath, "relativePath", relativePath)
	return detectedPath, relativePath, nil
}

// detectChartInCurrentDirectory first checks the given start directory ("."), then searches upwards within the provided filesystem for a Chart.yaml file.
// It returns the absolute path (relative to fs root) to the chart directory and a matching relative path,
// or an error if not found.
func detectChartInCurrentDirectory(fs afero.Fs) (detectedAbsPath, detectedRelPath string, err error) {
	startSearchDir := "."
	log.Debug("detectChartInCurrentDirectory: Start", "fs_root_relative_start", startSearchDir)

	// 1. Check the starting directory itself
	startChartFilePath := filepath.Join(startSearchDir, chartutil.ChartfileName)
	log.Debug("Checking for chart in start directory", "path", startChartFilePath)

	exists, err := afero.Exists(fs, startChartFilePath)
	if err != nil {
		log.Debug("Error checking for chart file existence in start dir (ignoring)", "path", startChartFilePath, "error", err)
	}
	if exists {
		cleanAbsPath := filepath.Clean(startSearchDir)
		log.Debug("Chart found in start directory", "absolutePath", cleanAbsPath)
		// Return the start directory path for both values when found immediately
		return cleanAbsPath, cleanAbsPath, nil
	}
	log.Debug("Chart not found in start directory, searching upwards...")

	// 2. Search upwards from the parent of the starting directory
	currentDir := filepath.Dir(startSearchDir) // Start searching from parent
	if currentDir == startSearchDir {          // Handle case where start is already root
		currentDir = "." // Ensure we check root if needed
	}

	maxSearchDepth := 100 // Prevent infinite loops

	for i := 0; i < maxSearchDepth; i++ {
		// If currentDir is empty or invalid, stop
		if currentDir == "" || currentDir == "/" || currentDir == "." && i > 0 { // Avoid redundant check of "." if we started there
			log.Debug("Reached root or invalid directory while searching upwards", "currentDir", currentDir)
			break
		}

		chartFilePath := filepath.Join(currentDir, chartutil.ChartfileName)
		log.Debug("Checking for chart upwards", "path", chartFilePath, "iteration", i)

		exists, err := afero.Exists(fs, chartFilePath)
		if err != nil {
			log.Debug("Error checking for chart file existence upwards (ignoring)", "path", chartFilePath, "error", err)
		}

		if exists {
			cleanAbsPath := filepath.Clean(currentDir)
			log.Debug("Chart found upwards", "absolutePath", cleanAbsPath)
			// Return the found path for both values
			return cleanAbsPath, cleanAbsPath, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir { // Termination check
			log.Debug("Reached filesystem root while searching upwards", "currentDir", currentDir)
			break
		}
		currentDir = parentDir
	}

	log.Debug("Chart not found searching upwards from fs root", "startDir", startSearchDir)
	return "", "", fmt.Errorf("no %s found in current directory or searching upwards from the root of the provided filesystem", chartutil.ChartfileName)
}

// processImagePatterns converts analyzer patterns to ImageInfo and identifies skipped patterns.
func processImagePatterns(patterns []analysis.ImagePattern) (images []ImageInfo, skipped []string) {
	for _, p := range patterns {
		imgInfo := ImageInfo{
			Source:    p.SourceOrigin, // Use SourceOrigin if available, else Path
			ValuePath: p.Path,         // Path represents the structural path in merged values
		}
		// If SourceOrigin is empty (e.g., from legacy analyzer), fallback to Path
		if imgInfo.Source == "" {
			imgInfo.Source = p.Path
		}

		// Determine registry based on pattern type
		var regStr string
		// Use a switch statement for clarity as suggested by gocritic
		switch {
		case p.Type == analysis.PatternTypeMap && p.Structure != nil:
			if regVal, ok := p.Structure["registry"].(string); ok {
				regStr = regVal
			}
		case p.Type == analysis.PatternTypeString && p.Structure != nil:
			if regVal, ok := p.Structure["registry"].(string); ok {
				regStr = regVal
			}
		default:
			// Attempt basic parsing if structure is missing or type is unexpected
			// Use a temporary variable to avoid shadowing
			imgRef, err := image.ParseImageReference(p.Value)
			if err == nil && imgRef != nil {
				regStr = imgRef.Registry
			}
		}

		// Add source registry to the set if it looks valid
		if regStr != "" {
			imgInfo.Registry = regStr
		}

		// Use a switch statement for clarity as suggested by gocritic
		switch p.Type {
		case analysis.PatternTypeMap:
			if p.Structure == nil {
				log.Warn("Skipping map pattern with nil structure", "path", p.Path, "value", p.Value)
				skipped = append(skipped, fmt.Sprintf("%s: %v (map type with nil structure)", p.Path, p.Value))
				continue
			}
			// For map types, use the pre-parsed structure directly
			// Use type assertion with ok check for safety
			if repoVal, ok := p.Structure["repository"].(string); ok {
				imgInfo.Repository = repoVal
			}
			if tagVal, ok := p.Structure["tag"].(string); ok {
				imgInfo.Tag = tagVal
			}
			log.Debug("processImagePatterns [MAP]: Using structure", "path", p.Path, "registry", imgInfo.Registry, "repo", imgInfo.Repository)

		case analysis.PatternTypeString:
			// For string types, parse the Value string using the correct function
			// Create a ChartMetadata object if SourceChartAppVersion is available
			var chartMetadata *image.ChartMetadata
			if p.SourceChartAppVersion != "" {
				chartMetadata = &image.ChartMetadata{
					AppVersion: p.SourceChartAppVersion,
				}
				log.Debug("processImagePatterns [STRING]: Using SourceChartAppVersion", "path", p.Path, "appVersion", p.SourceChartAppVersion)
			}

			// Pass the chartMetadata to ParseImageReference
			ref, err := image.ParseImageReference(p.Value, chartMetadata)
			if err != nil {
				log.Warn("Skipping string pattern due to parse error", "path", p.Path, "value", p.Value, "error", err)

				skipped = append(skipped, fmt.Sprintf("%s: %s (parse error: %v)", p.Path, p.Value, err))
				continue
			}

			// Populate from parsed reference
			// Note: Registry might be overwritten here if ParseImageReference finds one
			// and the earlier structure check didn't (e.g., for complex strings)
			imgInfo.Registry = ref.Registry
			imgInfo.Repository = ref.Repository
			imgInfo.Tag = ref.Tag
			imgInfo.Digest = ref.Digest
			log.Debug("processImagePatterns [STRING]: Parsed value", "path", p.Path, "value", p.Value, "registry", imgInfo.Registry, "repo", imgInfo.Repository, "tag", imgInfo.Tag)

		default:
			// Skip other types or maps without structure
			log.Warn("Skipping pattern with unhandled type", "path", p.Path, "type", p.Type, "value", p.Value)
			skipped = append(skipped, fmt.Sprintf("%s: %s (unhandled type: %s)", p.Path, p.Value, p.Type))
			continue
		}

		// Add original registry info if available from the pattern (context-aware only)
		if p.OriginalRegistry != "" {
			imgInfo.OriginalRegistry = p.OriginalRegistry
		}

		// Only add if we have a valid repository
		if imgInfo.Repository != "" {
			log.Debug("processImagePatterns: Adding ImageInfo", "path", p.Path, "finalRegistry", imgInfo.Registry, "finalRepo", imgInfo.Repository, "finalTag", imgInfo.Tag)
			images = append(images, imgInfo)
		} else {
			log.Warn("Skipping processed pattern due to empty repository", "path", p.Path, "type", p.Type, "value", p.Value)
			skipped = append(skipped, fmt.Sprintf("%s: %s (empty repository after processing)", p.Path, p.Value))
		}
	}
	return images, skipped
}

// filterImagesBySourceRegistries modifies the analysis object to only include images
// from the specified source registries.
func filterImagesBySourceRegistries(_ *cobra.Command, flags *InspectFlags, analysisResult *ImageAnalysis) {
	sourceSet := make(map[string]bool)
	for _, r := range flags.SourceRegistries {
		normalized := image.NormalizeRegistry(r)
		sourceSet[normalized] = true
	}

	if len(sourceSet) == 0 {
		log.Warn("No valid source registries provided for filtering.")
		return // No valid registries to filter by
	}

	filteredImages := make([]ImageInfo, 0, len(analysisResult.Images))
	for _, img := range analysisResult.Images {
		normalizedRegistry := image.NormalizeRegistry(img.Registry)
		if sourceSet[normalizedRegistry] {
			filteredImages = append(filteredImages, img)
		}
	}
	analysisResult.Images = filteredImages

	// Also filter imagePatterns (simple approach: remove if no resulting image matches)
	// A more robust approach might analyze pattern structure itself.
	filteredPatterns := make([]analysis.ImagePattern, 0, len(analysisResult.ImagePatterns))
	for _, pattern := range analysisResult.ImagePatterns {
		// Create ChartMetadata if pattern has SourceChartAppVersion
		var chartMetadata *image.ChartMetadata
		if pattern.SourceChartAppVersion != "" {
			chartMetadata = &image.ChartMetadata{
				AppVersion: pattern.SourceChartAppVersion,
			}
		}

		imgRef, err := image.ParseImageReference(pattern.Value, chartMetadata) // Pass chartMetadata
		if err == nil {
			normalizedRegistry := image.NormalizeRegistry(imgRef.Registry)
			if sourceSet[normalizedRegistry] {
				filteredPatterns = append(filteredPatterns, pattern)
			}
		} else {
			// Keep for now, as it might represent a template or complex structure.
			// log.Debug("Pattern value parsing failed, keeping pattern during filtering", "path", pattern.Path, "value", pattern.Value, "error", err)
			// Heuristic: Check if *any* part of the value string matches a source registry? Risky.
			// Let's keep patterns that don't parse cleanly for now.
			filteredPatterns = append(filteredPatterns, pattern)
		}
	}
	analysisResult.ImagePatterns = filteredPatterns
}

// getAllReleases returns all Helm releases across all namespaces
func getAllReleases() ([]*helm.ReleaseElement, *helm.Adapter, error) {
	// Create a Helm adapter for interacting with the cluster
	helmAdapter, err := helmAdapterFactory()
	if err != nil {
		return nil, nil, err // Assumes factory returns ExitCodeError on failure
	}
	// Add explicit nil check for helmAdapter to satisfy nilaway and prevent potential panics
	if helmAdapter == nil {
		return nil, nil, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitGeneralRuntimeError,
			Err:  errors.New("internal error: helmAdapterFactory returned nil adapter without error"),
		}
	}

	// List all releases across all namespaces
	client, err := createHelmClient()
	if err != nil {
		return nil, helmAdapter, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to create Helm client: %w", err),
		}
	}

	log.Debug("Listing all Helm releases across all namespaces")
	releases, err := client.ListReleases(context.Background(), true)
	if err != nil {
		return nil, helmAdapter, &exitcodes.ExitCodeError{
			Code: exitcodes.ExitHelmCommandFailed,
			Err:  fmt.Errorf("failed to list Helm releases: %w", err),
		}
	}

	log.Debug("Processing release", "name", releases[0].Name, "namespace", releases[0].Namespace)

	if len(releases) == 0 {
		log.Warn("No Helm releases found across all namespaces.")
	} else {
		log.Info(fmt.Sprintf("Found %d releases across all namespaces", len(releases)))
	}

	return releases, helmAdapter, nil
}

// processAllReleases iterates through all releases, analyzes them, and aggregates results.
func processAllReleases(releases []*helm.ReleaseElement, helmAdapter *helm.Adapter, flags *InspectFlags) ([]*ReleaseAnalysisResult, []string, []ImageInfo, error) {
	// Initialize return values
	var allResults []*ReleaseAnalysisResult
	var skippedReleases []string
	var allUnfilteredImages []ImageInfo // Will collect all images before filtering

	// Track unique registries for skeleton generation
	uniqueRegistries := make(map[string]bool)

	// Process each release
	for _, release := range releases {
		// Analyze the release
		result, unfilteredImages, err := analyzeRelease(release, helmAdapter, flags)
		if err != nil {
			log.Error("Error analyzing release", "release", release.Name, "namespace", release.Namespace, "error", err)
			skippedReleases = append(skippedReleases, fmt.Sprintf("%s/%s: %v", release.Namespace, release.Name, err))
			continue
		}

		// Add to results collection
		allResults = append(allResults, result)

		// Add images to the collection for skeleton
		// Create a new slice with enough capacity to avoid append problems
		if len(unfilteredImages) > 0 {
			newSlice := make([]ImageInfo, len(allUnfilteredImages), len(allUnfilteredImages)+len(unfilteredImages)+sliceGrowthBuffer)
			copy(newSlice, allUnfilteredImages)
			allUnfilteredImages = newSlice
			// Now safe to use append
			allUnfilteredImages = append(allUnfilteredImages, unfilteredImages...)
		}

		// Accumulate unique registries FROM THE UNFILTERED IMAGES for skeleton generation
		log.Debug("Processing release for skeleton registry aggregation", "release", release.Name, "namespace", release.Namespace, "unfiltered_image_count", len(unfilteredImages))
		for _, img := range unfilteredImages {
			log.Debug("SKELETON_CHECK: Checking ImageInfo for skeleton", "registry", img.Registry, "repository", img.Repository, "tag", img.Tag, "source", img.Source, "valuePath", img.ValuePath)
			if img.Registry != "" { // Ensure we don't add empty registries
				if !uniqueRegistries[img.Registry] {
					log.Debug("SKELETON_ADD: Adding potential unique registry to skeleton set", "registry", img.Registry)
				}
				uniqueRegistries[img.Registry] = true // Add registry to the map (will be filtered later)
			} else {
				log.Debug("SKELETON_SKIP: Skipping ImageInfo with empty registry", "repository", img.Repository, "source", img.Source)
			}
		}
	}

	// --- Filter uniqueRegistries for skeleton generation ---
	validatedRegistries := make(map[string]bool)
	log.Debug("Filtering collected unique registries for skeleton generation...")
	for registry := range uniqueRegistries {
		if isValidRegistryHostname(registry) {
			log.Debug("SKELETON_VALIDATE: Keeping valid registry hostname", "registry", registry)
			validatedRegistries[registry] = true
		} else {
			log.Debug("SKELETON_VALIDATE: Discarding invalid registry hostname", "registry", registry)
		}
	}
	log.Debug("Finished filtering registries", "initial_count", len(uniqueRegistries), "validated_count", len(validatedRegistries))

	// Create ImageInfo slice specifically for skeleton generation from VALIDATED registries
	var skeletonImages []ImageInfo
	for registry := range validatedRegistries { // Iterate the FILTERED map
		skeletonImages = append(skeletonImages, ImageInfo{
			Registry: registry, // Use the validated registry key
		})
	}

	// Return results, skipped releases, and the VALIDATED skeleton image list
	return allResults, skippedReleases, skeletonImages, nil
}

// checkSubchartDiscrepancy checks for discrepancies between the analyzer's image count
// and the images found in rendered chart templates (specifically from Deployments and StatefulSets).
// It returns an error only for fatal issues like chart loading errors, not for discrepancies.
func checkSubchartDiscrepancy(cmd *cobra.Command, chartPath string, analysisResult *ImageAnalysis) error {
	log.Debug("Checking for subchart image discrepancies")

	// Get values files from command line
	valueOpts := &values.Options{}
	valuesFiles, err := cmd.Flags().GetStringSlice("values")
	if err != nil {
		return fmt.Errorf("failed to get values files: %w", err)
	}
	valueOpts.ValueFiles = valuesFiles

	// Load the chart
	loadedChart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart for subchart check: %w", err)
	}

	// Read values from files
	vals := map[string]interface{}{}
	for _, valueFile := range valueOpts.ValueFiles {
		currentValues, err := chartutil.ReadValuesFile(valueFile)
		if err != nil {
			return fmt.Errorf("failed to read values file %s: %w", valueFile, err)
		}
		// Merge with existing values
		vals = chartutil.CoalesceTables(vals, currentValues.AsMap())
	}

	// Merge with chart's default values
	vals = chartutil.CoalesceTables(loadedChart.Values, vals)

	// Render chart templates
	actionConfig := new(action.Configuration)
	installAction := action.NewInstall(actionConfig)
	installAction.DryRun = true
	installAction.ReleaseName = "irr-subchart-check"
	installAction.Namespace = validateTestNamespace
	installAction.ClientOnly = true

	// Render the templates
	release, err := installAction.Run(loadedChart, vals)
	if err != nil {
		log.Warn("Failed to render chart templates for subchart check, skipping", "chart", loadedChart.Name(), "error", err)
		return nil // Return nil to indicate non-fatal error for this check
	}

	// Check if the release object itself is nil (can happen in dry-run)
	if release == nil {
		log.Warn("Chart rendering resulted in a nil release object, skipping subchart check", "chart", loadedChart.Name())
		return nil
	}

	// Add check for empty manifest before processing
	if release.Manifest == "" {
		log.Warn("Rendered release has an empty manifest, skipping subchart discrepancy check", "chart", loadedChart.Name())
		return nil
	}

	// Extract images from rendered templates
	templateImages := make(map[string]struct{})
	manifests := release.Manifest

	// Split manifests into separate YAML documents
	decoder := yaml.NewDecoder(strings.NewReader(manifests))
	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			// If we've reached the end of the documents, break
			if err.Error() == "EOF" {
				break
			}
			// Log parsing errors as warnings but continue with other documents
			log.Warn("Error parsing rendered template document: %s", err)
			continue
		}

		// Skip empty documents
		if len(doc) == 0 {
			continue
		}

		// Check if this is a Deployment or StatefulSet
		kind, ok := doc["kind"].(string)
		if !ok || (kind != "Deployment" && kind != "StatefulSet") {
			continue
		}

		// Extract images using safe traversal
		extractImagesFromResource(doc, templateImages)
	}

	// Compare image counts
	analyzerImageCount := len(analysisResult.Images)
	templateImageCount := len(templateImages)

	// Circuit breaker check - using constant instead of magic number
	const maxImageThreshold = 300
	if templateImageCount > maxImageThreshold {
		log.Debug("Template image count exceeds threshold (%d), skipping comparison", templateImageCount)
		return nil
	}

	// Issue warning if counts differ
	if analyzerImageCount != templateImageCount {
		log.Warn("Subchart image discrepancy detected",
			"check", "subchart_discrepancy",
			"analyzer_image_count", analyzerImageCount,
			"template_image_count", templateImageCount,
			"message", "The analyzer found different number of images than the rendered templates. "+
				"This may indicate images defined in subchart default values that were not detected. "+
				"Consider using the --no-subchart-check flag to skip this check.")
	}

	return nil
}

// extractImagesFromResource safely extracts image references from a Kubernetes resource.
// It traverses the resource structure to find container image fields in pods.
func extractImagesFromResource(resource map[string]interface{}, images map[string]struct{}) {
	// Try to get to spec.template.spec for pod template
	spec, ok := resource["spec"].(map[string]interface{})
	if !ok {
		return
	}

	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return
	}

	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return
	}

	// Extract images from containers
	extractImagesFromContainers(podSpec, "containers", images)

	// Extract images from initContainers
	extractImagesFromContainers(podSpec, "initContainers", images)
}

// extractImagesFromContainers extracts image references from container lists
func extractImagesFromContainers(podSpec map[string]interface{}, containerType string, images map[string]struct{}) {
	containers, ok := podSpec[containerType].([]interface{})
	if !ok {
		return
	}

	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		imageValue, ok := container["image"].(string)
		if !ok || imageValue == "" {
			continue
		}

		// Add image to the set
		images[imageValue] = struct{}{}
	}
}
