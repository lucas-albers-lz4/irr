// Package main declares constants used across the irr command-line interface.
package main

// Chart source type constants
const (
	// ChartSourceTypeChart indicates the chart source is a chart path
	ChartSourceTypeChart = "chart"
	// ChartSourceTypeRelease indicates the chart source is a release name
	ChartSourceTypeRelease = "release"
	// ChartSourceTypeAutoDetected indicates the chart source was auto-detected
	ChartSourceTypeAutoDetected = "auto-detected"

	cliName = "irr"
)

// Helm plugin environment variable names.
const (
	envHelmPluginDir          = "HELM_PLUGIN_DIR"
	envHelmPluginName         = "HELM_PLUGIN_NAME"
	envHelmNamespace          = "HELM_NAMESPACE"
	envHelmBin                = "HELM_BIN"
	envHelmDebug              = "HELM_DEBUG"
	envHelmPlugins            = "HELM_PLUGINS"
	envHelmRegistryConfig     = "HELM_REGISTRY_CONFIG"
	envHelmRepositoryCache    = "HELM_REPOSITORY_CACHE"
	envHelmRepositoryConfig   = "HELM_REPOSITORY_CONFIG"
)
