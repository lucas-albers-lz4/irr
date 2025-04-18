package helm

import (
	"fmt"
	"os"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/strvals"

	"github.com/lalbers/irr/pkg/fileutil"
	log "github.com/lalbers/irr/pkg/log"
	"sigs.k8s.io/yaml"
)

// HelmTemplateFunc allows overriding the Template function for testing
var HelmTemplateFunc = Template

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
	KubeVersion string
	Strict      bool
}

// GetValuesOptions represents options for helm get values command
type GetValuesOptions struct {
	ReleaseName string
	Namespace   string
	OutputFile  string
}

// Template executes the helm template command with the given options
func Template(options *TemplateOptions) (*CommandResult, error) {
	// Initialize Helm environment settings and action config
	settings := cli.New()
	actionConfig := new(action.Configuration)
	// Use an empty namespace initially, let Helm determine default or use provided
	if err := actionConfig.Init(settings.RESTClientGetter(), options.Namespace, "", log.Infof); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	// Create install action for templating
	install := action.NewInstall(actionConfig)
	install.ReleaseName = options.ReleaseName
	install.Version = ""        // Process latest version unless specified
	install.DryRun = true       // Perform a template operation
	install.ClientOnly = true   // Do not connect to a cluster
	install.IncludeCRDs = false // Typically not needed for simple validation

	// Log if strict mode is enabled
	if options.Strict {
		log.Debugf("Using strict mode for templating")
		// Note: Helm SDK doesn't support strict mode via action.Install directly
		// We'll implement our own strict validation after the template is generated
	}

	// Set namespace if provided
	if options.Namespace != "" {
		install.Namespace = options.Namespace
	} else {
		install.Namespace = settings.Namespace() // Use default namespace from settings
	}

	// Set Kubernetes Version if provided
	if options.KubeVersion != "" {
		kubeVersion, err := chartutil.ParseKubeVersion(options.KubeVersion)
		if err != nil {
			return nil, fmt.Errorf("invalid Kubernetes version %q: %w", options.KubeVersion, err)
		}
		install.KubeVersion = kubeVersion
		log.Debugf("Using Kubernetes version for templating: %s", options.KubeVersion)
	}

	// Load chart values
	values, err := mergeValues(options.ValuesFiles, options.SetValues)
	if err != nil {
		return nil, fmt.Errorf("failed to merge values: %w", err)
	}

	// Load the chart
	chartRequested, err := loader.Load(options.ChartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from path %q: %w", options.ChartPath, err)
	}

	// Execute the template action
	rel, err := install.Run(chartRequested, values)
	if err != nil {
		// Attempt to provide more specific error context if possible
		errorMsg := fmt.Sprintf("Helm template failed for chart %q with release name %q", options.ChartPath, options.ReleaseName)
		if rel != nil && rel.Info != nil {
			errorMsg += fmt.Sprintf(" (status: %s)", rel.Info.Status)
		}
		return nil, fmt.Errorf("%s: %w", errorMsg, err)
	}

	// Return successful result
	return &CommandResult{
		Success: true,
		Stdout:  rel.Manifest,
		Stderr:  "", // Helm SDK Run doesn't typically populate Stderr on success
	}, nil
}

// GetValues executes the helm get values command with the given options
func GetValues(options *GetValuesOptions) (*CommandResult, error) {
	settings := cli.New()
	actionConfig := new(action.Configuration)
	ns := options.Namespace
	if ns == "" {
		ns = settings.Namespace() // Use default if not specified
	}

	if err := actionConfig.Init(settings.RESTClientGetter(), ns, "", log.Infof); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action config: %w", err)
	}

	getValuesAction := action.NewGetValues(actionConfig)
	// Configure output format through appropriate method or property if needed

	log.Infof("Executing helm get values for release %q in namespace %q", options.ReleaseName, ns)
	values, err := getValuesAction.Run(options.ReleaseName)
	if err != nil {
		return nil, fmt.Errorf("helm get values failed for release %q: %w", options.ReleaseName, err)
	}

	// Convert values map to YAML string
	yamlData, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal release values to YAML: %w", err)
	}

	// Write to file if specified
	if options.OutputFile != "" {
		if err := os.WriteFile(options.OutputFile, yamlData, fileutil.ReadWriteUserPermission); err != nil {
			return nil, fmt.Errorf("failed to write values to output file %q: %w", options.OutputFile, err)
		}
		log.Infof("Release values written to %s", options.OutputFile)
		// Return success but no stdout as it went to file
		return &CommandResult{Success: true, Stdout: "", Stderr: ""}, nil
	}

	return &CommandResult{
		Success: true,
		Stdout:  string(yamlData),
		Stderr:  "",
	}, nil
}

// Helper function to merge values from files and set flags
// This replicates part of the logic previously in runValidate and Helm's internal handling
func mergeValues(valueFiles, setValues []string) (map[string]interface{}, error) {
	base := map[string]interface{}{}

	// Load values from files first
	for _, filePath := range valueFiles {
		currentMap := map[string]interface{}{}

		// Validate file exists before reading to provide better error
		if _, err := os.Stat(filePath); err != nil {
			return nil, fmt.Errorf("values file %q not accessible: %w", filePath, err)
		}

		// Read and parse the file
		bytes, err := os.ReadFile(filePath) // #nosec G304 - This is a deliberately provided values file path
		if err != nil {
			return nil, fmt.Errorf("failed reading values file %s: %w", filePath, err)
		}

		if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
			return nil, fmt.Errorf("failed unmarshalling values file %s: %w", filePath, err)
		}
		// Merge with existing values
		base = chartutil.CoalesceTables(base, currentMap)
	}

	// Apply --set values overrides
	for _, value := range setValues {
		if err := strvals.ParseInto(value, base); err != nil {
			return nil, fmt.Errorf("failed parsing --set data %q: %w", value, err)
		}
	}
	return base, nil
}
