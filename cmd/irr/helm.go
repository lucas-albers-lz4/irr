package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

// GetChartPathFromRelease attempts to get the chart path from a Helm release
// #nosec G204 - Command arguments are controlled by the code
func GetChartPathFromRelease(releaseName string) (string, error) {
	if releaseName == "" {
		return "", fmt.Errorf("release name is empty")
	}

	// Run helm get manifest command to get chart info
	cmd := exec.Command("helm", "get", "manifest", releaseName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get manifest for release %s: %w", releaseName, err)
	}

	// Parse output to find chart name and version
	chartInfo := parseChartInfo(string(output))
	if chartInfo.Name == "" {
		return "", fmt.Errorf("could not determine chart name from release %s", releaseName)
	}

	log.Infof("Found chart %s version %s for release %s", chartInfo.Name, chartInfo.Version, releaseName)

	// Create temporary directory to download chart
	tempDir, err := os.MkdirTemp("", "irr-helm-release-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Use helm to pull the chart to the temp directory
	// #nosec G204 - Command arguments are controlled by the code
	chartURL := fmt.Sprintf("%s-%s.tgz", chartInfo.Name, chartInfo.Version)
	pullCmd := exec.Command("helm", "pull", chartURL, "--destination", tempDir)
	if err := pullCmd.Run(); err != nil {
		// Try to pull using a repository search
		// #nosec G204 - Command arguments are controlled by the code
		searchCmd := exec.Command("helm", "search", "repo", chartInfo.Name, "-o", "json")
		searchOutput, searchErr := searchCmd.CombinedOutput()
		if searchErr != nil {
			return "", fmt.Errorf("failed to search for chart %s: %w", chartInfo.Name, searchErr)
		}

		// Parse search results to find chart location
		// This is simplified and would need to be expanded for robust implementation
		chartRepo := parseChartRepo(string(searchOutput), chartInfo.Name)
		if chartRepo != "" {
			// #nosec G204 - Command arguments are controlled by the code
			pullCmd = exec.Command("helm", "pull", chartRepo+"/"+chartInfo.Name, "--version", chartInfo.Version, "--destination", tempDir)
			if err := pullCmd.Run(); err != nil {
				return "", fmt.Errorf("failed to pull chart %s: %w", chartInfo.Name, err)
			}
		} else {
			return "", fmt.Errorf("failed to pull chart %s and could not determine repository: %w", chartInfo.Name, err)
		}
	}

	// Find chart file in temp directory
	chartFile := fmt.Sprintf("%s/%s-%s.tgz", tempDir, chartInfo.Name, chartInfo.Version)
	if _, err := os.Stat(chartFile); err != nil {
		// Try to find any .tgz file
		files, err := os.ReadDir(tempDir)
		if err != nil {
			return "", fmt.Errorf("failed to read temp directory: %w", err)
		}

		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".tgz") {
				chartFile = tempDir + "/" + file.Name()
				break
			}
		}
	}

	// Return the path to the chart file
	if _, err := os.Stat(chartFile); err != nil {
		return "", fmt.Errorf("chart file not found: %w", err)
	}

	return chartFile, nil
}

// HelmChartInfo represents basic chart information
type HelmChartInfo struct {
	Name    string
	Version string
}

// parseChartInfo parses chart name and version from helm manifest output
func parseChartInfo(manifest string) HelmChartInfo {
	info := HelmChartInfo{}

	// Find chart: line
	lines := strings.Split(manifest, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "chart:") {
			// Split on first colon
			const splitLimit = 2
			parts := strings.SplitN(line, ":", splitLimit)
			if len(parts) == splitLimit {
				chartInfo := strings.TrimSpace(parts[1])
				// Parse chart info - could be 'name-version' or just 'name'
				chartParts := strings.Split(chartInfo, "-")
				if len(chartParts) > 1 {
					info.Name = strings.Join(chartParts[:len(chartParts)-1], "-")
					info.Version = chartParts[len(chartParts)-1]
				} else {
					info.Name = chartInfo
					info.Version = "latest"
				}
				break
			}
		}
	}

	return info
}

// parseChartRepo parses chart repository from helm search output
func parseChartRepo(searchOutput, chartName string) string {
	// This is a simplified implementation
	// For a robust implementation, proper JSON parsing would be needed
	if strings.Contains(searchOutput, chartName) {
		// Just return a placeholder - we would need to parse JSON properly
		return "stableRepo"
	}
	return ""
}
