package main

import (
	"fmt"
	"os"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"

	"github.com/lalbers/irr/pkg/helm"
	log "github.com/lalbers/irr/pkg/log"
	"github.com/spf13/cobra"
)

// addReleaseFlag adds a --release-name flag to the given command
func addReleaseFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String("release-name", "", "Helm release name to use for loading chart information")
}

// initHelmPlugin initializes the CLI with Helm plugin specific functionality
func initHelmPlugin() {
	// Add release-name flag to the root command
	addReleaseFlag(rootCmd)
}

// HelmChartInfo represents basic chart information
type HelmChartInfo struct {
	Name    string
	Version string
}

// GetChartPathFromRelease attempts to get the chart path from a Helm release
func GetChartPathFromRelease(releaseName string) (string, error) {
	if releaseName == "" {
		return "", fmt.Errorf("release name is empty")
	}

	// Initialize Helm environment
	settings := cli.New()

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
		repoManager := helm.NewHelmRepositoryManager(settings)
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
	if _, err := os.Stat(chartPath); err != nil {
		return "", fmt.Errorf("chart file not found: %w", err)
	}

	return chartPath, nil
}
