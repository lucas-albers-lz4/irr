package helm

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"

	log "github.com/lalbers/irr/pkg/log"
)

// CommandResult represents the result of a Helm command execution
type CommandResult struct {
	Success bool
	Stdout  string
	Stderr  string
	Error   error
}

// TemplateOptions represents options for helm template command
type TemplateOptions struct {
	ReleaseName string
	ChartPath   string
	ValuesFiles []string
	SetValues   []string
	Namespace   string
}

// GetValuesOptions represents options for helm get values command
type GetValuesOptions struct {
	ReleaseName string
	Namespace   string
	OutputFile  string
}

// Template executes the helm template command with the given options
func Template(options *TemplateOptions) (*CommandResult, error) {
	// Build helm template command
	helmArgs := []string{"template", options.ReleaseName, options.ChartPath}

	// Add values files
	for _, valueFile := range options.ValuesFiles {
		helmArgs = append(helmArgs, "--values", valueFile)
	}

	// Add set values
	for _, setValue := range options.SetValues {
		helmArgs = append(helmArgs, "--set", setValue)
	}

	// Add namespace if specified
	if options.Namespace != "" {
		helmArgs = append(helmArgs, "--namespace", options.Namespace)
	}

	return executeHelmCommand(helmArgs)
}

// GetValues executes the helm get values command with the given options
func GetValues(options *GetValuesOptions) (*CommandResult, error) {
	// Build helm get values command
	helmArgs := []string{"get", "values", options.ReleaseName}

	// Add namespace if specified
	if options.Namespace != "" {
		helmArgs = append(helmArgs, "--namespace", options.Namespace)
	}

	// Add output format
	helmArgs = append(helmArgs, "-o", "yaml")

	return executeHelmCommand(helmArgs)
}

// executeHelmCommand executes a helm command with the given arguments using the Helm SDK
func executeHelmCommand(args []string) (*CommandResult, error) {
	log.Infof("Executing: helm %s", strings.Join(args, " "))

	// Initialize Helm environment
	settings := cli.New()

	// Create action config
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), settings.Namespace(), "", log.Infof); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Create appropriate action based on command
	var result *CommandResult
	switch args[0] {
	case "template":
		install := action.NewInstall(actionConfig)
		install.ReleaseName = args[1]
		install.Namespace = settings.Namespace()
		install.DryRun = true
		install.ClientOnly = true

		// Load the chart
		chartPath := args[2]
		chart, err := loader.Load(chartPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load chart: %w", err)
		}

		// Execute template
		rel, err := install.Run(chart, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to template chart: %w", err)
		}

		result = &CommandResult{
			Success: true,
			Stdout:  rel.Manifest,
			Stderr:  "",
		}

	case "get":
		get := action.NewGet(actionConfig)
		rel, err := get.Run(args[1])
		if err != nil {
			return nil, fmt.Errorf("failed to get release: %w", err)
		}

		result = &CommandResult{
			Success: true,
			Stdout:  rel.Manifest,
			Stderr:  "",
		}

	default:
		return nil, fmt.Errorf("unsupported Helm command: %s", args[0])
	}

	return result, nil
}
