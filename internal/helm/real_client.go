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
	// Ensure actionConfig is initialized
	if c.actionConfig == nil {
		if err := c.initializeActionConfig(); err != nil {
			return nil, fmt.Errorf("helm action config initialization failed: %w", err)
		}
	}

	listAction := action.NewList(c.actionConfig)
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
func (c *RealHelmClient) initializeActionConfig() error {
	if c.actionConfig == nil {
		log.Debug("Initializing Helm action config...")
		c.actionConfig = new(action.Configuration)

		// Use the stored settings if available
		if c.settings == nil {
			log.Warn("Helm settings not available during action config initialization, using defaults")
			c.settings = cli.New()
		}

		// Create a logger function for Helm
		helmLogger := func(format string, args ...interface{}) {
			logMsg := fmt.Sprintf(format, args...)
			log.Debug("[Helm SDK] " + logMsg)
		}

		// Initialize the action config
		if err := c.actionConfig.Init(c.settings.RESTClientGetter(), c.settings.Namespace(), os.Getenv("HELM_DRIVER"), helmLogger); err != nil {
			return fmt.Errorf("failed to init helm action config: %w", err)
		}
	}
	return nil
}

// isReleaseNotFound checks if the error indicates a Helm release was not found.
// Using the correct driver package from helm.sh/helm/v3/pkg/storage/driver
// func isReleaseNotFound(err error) bool {
// 	return err != nil && (strings.Contains(err.Error(), "release: not found") || errors.Is(err, driver.ErrReleaseNotFound))
// }
