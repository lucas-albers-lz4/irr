package main

import (
	"context"
	"fmt"
	"os"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lalbers/irr/pkg/helm"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// Only keep a single definition of validateTestNamespace at the top of the file
const validateTestNamespace = "default"

// addReleaseFlag adds a --release-name flag to the given command
func addReleaseFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String("release-name", "", "Helm release name to use for loading chart information")
}

// addNamespaceFlag adds a --namespace flag to the given command if it doesn't already exist
func addNamespaceFlag(cmd *cobra.Command) {
	if cmd.PersistentFlags().Lookup("namespace") == nil {
		cmd.PersistentFlags().String("namespace", "", "Namespace for the Helm release (overrides the namespace from Helm environment)")
	}
}

// initHelmPlugin initializes the CLI with Helm plugin specific functionality
func initHelmPlugin() {
	// Ensure release-name and namespace flags are visible in plugin mode
	if releaseFlag := rootCmd.PersistentFlags().Lookup("release-name"); releaseFlag != nil {
		releaseFlag.Hidden = false
	}

	if namespaceFlag := rootCmd.PersistentFlags().Lookup("namespace"); namespaceFlag != nil {
		namespaceFlag.Hidden = false
	}

	// Set up any other Helm-specific flags or functionality
	log.Debugf("Helm plugin flags initialized")
}

// removeHelmPluginFlags removes plugin-specific flags from the root command
// This is used to ensure the root command doesn't have these flags in standalone mode
func removeHelmPluginFlags(cmd *cobra.Command) {
	if err := cmd.PersistentFlags().MarkHidden("release-name"); err != nil {
		log.Warnf("Failed to mark release-name flag as hidden: %v", err)
	}
	if err := cmd.PersistentFlags().MarkHidden("namespace"); err != nil {
		log.Warnf("Failed to mark namespace flag as hidden: %v", err)
	}
}

// HelmChartInfo represents basic chart information
type HelmChartInfo struct {
	Name    string
	Version string
}

// GetReleaseNamespace gets the namespace for a release
// Order of precedence:
// 1. --namespace flag
// 2. HELM_NAMESPACE environment variable
// 3. Default namespace ("default")
func GetReleaseNamespace(cmd *cobra.Command) string {
	namespace, err := cmd.Flags().GetString("namespace")
	if err == nil && namespace != "" {
		return namespace
	}

	// Check HELM_NAMESPACE env var
	envNamespace := os.Getenv("HELM_NAMESPACE")
	if envNamespace != "" {
		return envNamespace
	}

	// Return default namespace using string literal
	return validateTestNamespace
}

// GetHelmSettings returns the Helm CLI settings
func GetHelmSettings() *cli.EnvSettings {
	settings := cli.New()
	return settings
}

// GetChartPathFromRelease attempts to get the chart path from a Helm release
func GetChartPathFromRelease(releaseName string) (string, error) {
	if releaseName == "" {
		return "", fmt.Errorf("release name is empty")
	}

	// Initialize Helm environment
	settings := GetHelmSettings()

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", log.Infof); err != nil {
		return "", fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Create get action
	get := action.NewGet(actionConfig)

	// Get the release
	rel, err := get.Run(releaseName)
	if err != nil {
		return "", fmt.Errorf("failed to get release %s: %w", releaseName, err)
	}

	// Extract chart info
	chartInfo := HelmChartInfo{
		Name:    rel.Chart.Metadata.Name,
		Version: rel.Chart.Metadata.Version,
	}

	log.Infof("Found chart %s version %s for release %s", chartInfo.Name, chartInfo.Version, releaseName)

	// Create temp directory for chart
	tempDir, err := os.MkdirTemp("", "irr-chart-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Create pull action
	pull := action.NewPull()
	pull.Settings = settings
	pull.DestDir = tempDir
	pull.Version = chartInfo.Version

	// Try to pull the chart
	chartPath, err := pull.Run(chartInfo.Name)
	if err != nil {
		// Try to find the chart in repositories
		repoManager := helm.NewRepositoryManager(settings)
		chartRepo, err := repoManager.FindChartInRepositories(chartInfo.Name)
		if err != nil {
			return "", fmt.Errorf("failed to find chart %s in any repository: %w", chartInfo.Name, err)
		}

		// Try to pull with repository name
		chartPath, err = pull.Run(fmt.Sprintf("%s/%s", chartRepo, chartInfo.Name))
		if err != nil {
			return "", fmt.Errorf("failed to pull chart %s: %w", chartInfo.Name, err)
		}
	}

	// Return the path to the chart file
	if _, err := AppFs.Stat(chartPath); err != nil {
		return "", fmt.Errorf("chart file not found: %w", err)
	}

	return chartPath, nil
}

// GetReleaseValues retrieves the values from a Helm release
func GetReleaseValues(_ context.Context, releaseName, namespace string) (map[string]interface{}, error) {
	if releaseName == "" {
		return nil, fmt.Errorf("release name is empty")
	}

	// Initialize Helm environment
	settings := GetHelmSettings()

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, "", log.Infof); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Create get values action
	getValues := action.NewGetValues(actionConfig)
	getValues.AllValues = true

	// Get the values
	vals, err := getValues.Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get values for release %s: %w", releaseName, err)
	}

	return vals, nil
}
