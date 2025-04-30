// Package helm provides internal utilities for interacting with Helm.
package helm

import (
	"context"
	"fmt"
	"os"

	log "github.com/lucas-albers-lz4/irr/pkg/log"
	"helm.sh/helm/v3/pkg/action"
	helmChart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
)

// LoadChart loads a Helm chart from the specified path using the actual Helm loader.
func (c *RealHelmClient) LoadChart(chartPath string) (*helmChart.Chart, error) {
	chart, err := loader.Load(chartPath)
	if err != nil {
		// Return a standard error type, potentially wrapped if more context is needed
		return nil, fmt.Errorf("failed to load chart '%s': %w", chartPath, err)
	}
	return chart, nil
}

// ListReleases lists Helm releases using the actual Helm SDK.
func (c *RealHelmClient) ListReleases(_ context.Context, allNamespaces bool) ([]*ReleaseElement, error) {
	log.Debug("Listing releases", "allNamespaces", allNamespaces)

	// Create a new action config for this specific list operation
	actionConfig := new(action.Configuration)

	// Determine the namespace for initialization
	// If listing all namespaces, initialize with an empty namespace string.
	// Otherwise, use the namespace from settings.
	initNamespace := c.settings.Namespace()
	if allNamespaces {
		log.Debug("Initializing Helm action config for all namespaces (namespace=\"\")")
		initNamespace = ""
	} else {
		log.Debug("Initializing Helm action config for specific namespace", "namespace", initNamespace)
	}

	// Create a logger function for Helm
	helmLogger := func(format string, args ...interface{}) {
		logMsg := fmt.Sprintf(format, args...)
		log.Debug("[Helm SDK] " + logMsg)
	}

	// Initialize the action config for this specific operation
	if err := actionConfig.Init(c.settings.RESTClientGetter(), initNamespace, os.Getenv("HELM_DRIVER"), helmLogger); err != nil {
		return nil, fmt.Errorf("failed to init helm action config for ListReleases: %w", err)
	}

	// Create and configure the list action
	listAction := action.NewList(actionConfig) // Use the specifically initialized config
	listAction.AllNamespaces = allNamespaces
	listAction.SetStateMask() // List deployed and failed states by default
	log.Debug("Running Helm list action", "allNamespaces", allNamespaces)

	results, err := listAction.Run()
	if err != nil {
		log.Error("Helm list action failed", "error", err)
		return nil, fmt.Errorf("failed to list Helm releases: %w", err)
	}
	log.Debug("Helm list action completed", "count", len(results))

	// Convert []*release.Release to []*ReleaseElement
	releases := make([]*ReleaseElement, 0, len(results))
	for _, rel := range results {
		if rel == nil {
			log.Debug("Skipping nil release in list results")
			continue
		}
		releases = append(releases, &ReleaseElement{
			Name:      rel.Name,
			Namespace: rel.Namespace,
		})
	}

	return releases, nil
}

// initializeActionConfig ensures the actionConfig is ready.
// NOTE: This might be less relevant now if ListReleases initializes its own config.
func (c *RealHelmClient) initializeActionConfig() error {
	// If GetReleaseValues/GetChartFromRelease are calling this after setting c.settings.Namespace,
	// then this initialization *should* pick up the correct temporary namespace.
	// We log the namespace being used here to confirm.
	currentNamespaceSetting := c.settings.Namespace()
	log.Debug("Initializing/Re-initializing shared Helm action config", "namespace_used", currentNamespaceSetting)

	if c.actionConfig == nil {
		log.Debug("Action config was nil, creating new.")
		c.actionConfig = new(action.Configuration)
	} else {
		log.Debug("Action config exists, re-initializing.")
		// Potentially clear or reset parts of actionConfig if re-init is needed, although Init should handle it.
	}

	// Use the stored settings if available
	if c.settings == nil {
		log.Warn("Helm settings not available during action config initialization, using defaults")
		c.settings = cli.New()
		currentNamespaceSetting = c.settings.Namespace() // Update if settings were just created
		log.Debug("Default settings created", "default_namespace", currentNamespaceSetting)
	}

	// Create a logger function for Helm
	helmLogger := func(format string, args ...interface{}) {
		logMsg := fmt.Sprintf(format, args...)
		log.Debug("[Helm SDK] " + logMsg)
	}

	// Initialize the action config using the current namespace from settings
	if err := c.actionConfig.Init(c.settings.RESTClientGetter(), currentNamespaceSetting, os.Getenv("HELM_DRIVER"), helmLogger); err != nil {
		return fmt.Errorf("failed to init/re-init helm action config (ns: %s): %w", currentNamespaceSetting, err)
	}
	log.Debug("Action config initialized/re-initialized successfully", "namespace_used", currentNamespaceSetting)
	return nil
}

// isReleaseNotFound checks if the error indicates a Helm release was not found.
// Using the correct driver package from helm.sh/helm/v3/pkg/storage/driver
// func isReleaseNotFound(err error) bool {
// 	return err != nil && (strings.Contains(err.Error(), "release: not found") || errors.Is(err, driver.ErrReleaseNotFound))
// }
