package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lalbers/irr/pkg/chart"
	"github.com/lalbers/irr/pkg/debug"
	"github.com/lalbers/irr/pkg/image"
	"github.com/lalbers/irr/pkg/registry"
	"github.com/lalbers/irr/pkg/strategy"
	"github.com/spf13/cobra"
)

// Package main provides the helm-image-override command line tool.
// This tool helps migrate container images between registries by generating
// Helm value overrides that redirect image references to a new registry.

// Key concepts:
// - Chart: A Helm chart that contains container image references
// - Registry: A container registry (e.g., docker.io, quay.io)
// - Override: A YAML file that overrides chart values
// - Path Strategy: How image paths are transformed

// Command line flags:
// Required:
// --chart-path: Path to the Helm chart
// --target-registry: Target registry for images
// --source-registries: Source registries to process
//
// Optional:
// --output-file: Output file for overrides (default: stdout)
// --path-strategy: Strategy for path transformation
// --verbose: Enable verbose output
// --dry-run: Preview changes
// --strict: Fail on unrecognized structures
// --exclude-registries: Registries to skip
// --threshold: Success threshold percentage
// --debug: Enable debug logging
// --registry-mappings: Path to YAML file containing registry mappings

// @llm-helper This tool uses the Helm SDK to load and process charts
// @llm-helper Image detection is done through reflection and pattern matching
// @llm-helper YAML output is carefully formatted for Helm compatibility

// Exit codes for different error conditions
const (
	ExitSuccess                 = 0 // Successful execution
	ExitGeneralRuntimeError     = 1 // General runtime error
	ExitInputConfigurationError = 2 // Invalid input configuration
	ExitChartParsingError       = 3 // Error parsing Helm chart
	ExitImageProcessingError    = 4 // Error processing images
	ExitUnsupportedStructError  = 5 // Unsupported structure found
	ExitThresholdNotMetError    = 6 // Success threshold not met
	ExitCodeInvalidStrategy     = 7 // Invalid path strategy
	ExitHelmTemplateError       = 8 // Helm template validation failed
)

// Command line flag variables
var (
	chartPath            string // Path to the Helm chart
	targetRegistry       string // Target registry URL
	sourceRegistries     string // Comma-separated source registries
	outputFile           string // Output file path
	pathStrategy         string // Path transformation strategy
	verbose              bool   // Enable verbose output
	dryRun               bool   // Preview mode
	strictMode           bool   // Strict validation mode
	excludeRegistries    string // Registries to exclude
	threshold            int    // Success threshold
	debugEnabled         bool   // Debug logging
	registryMappingsFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "irr",
		Short: "Tool for generating Helm overrides to redirect container images",
		Long: `IRR (Image Registry Rewrite) helps migrate container images between registries by generating
Helm value overrides that redirect image references to a new registry.`,
		// Disable automatic printing of usage on error
		SilenceUsage: true,
		// Disable automatic printing of errors
		SilenceErrors: true,
	}

	// Add the default command (existing functionality)
	rootCmd.AddCommand(newDefaultCmd())

	// Add the analyze command
	rootCmd.AddCommand(newAnalyzeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newDefaultCmd creates the default command that provides the original functionality
func newDefaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "override [flags]",
		Short: "Generate Helm overrides for redirecting container images",
		Long: `Generate Helm value overrides that redirect container images from source registries
to a target registry. This is the original functionality of IRR (Image Registry Rewrite).`,
		RunE: runDefault,
		// Disable automatic printing of usage on error
		SilenceUsage: true,
		// Disable automatic printing of errors
		SilenceErrors: true,
	}

	// Add all the original flags
	f := cmd.Flags()
	f.StringVar(&chartPath, "chart-path", "", "Path to the Helm chart (directory or .tgz archive)")
	f.StringVar(&targetRegistry, "target-registry", "", "Target registry URL (e.g., harbor.example.com:5000)")
	f.StringVar(&sourceRegistries, "source-registries", "", "Comma-separated list of source registries to rewrite (e.g., docker.io,quay.io)")
	f.StringVar(&outputFile, "output-file", "", "Output file path for overrides (default: stdout)")
	f.StringVar(&pathStrategy, "path-strategy", "prefix-source-registry", "Path strategy to use (currently only prefix-source-registry is supported)")
	f.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	f.BoolVar(&dryRun, "dry-run", false, "Preview changes without writing file")
	f.BoolVar(&strictMode, "strict", false, "Fail on unrecognized image structures")
	f.StringVar(&excludeRegistries, "exclude-registries", "", "Comma-separated list of registries to exclude from processing")
	f.IntVar(&threshold, "threshold", 100, "Success threshold percentage (0-100)")
	f.BoolVar(&debugEnabled, "debug", false, "Enable debug logging")
	f.StringVar(&registryMappingsFile, "registry-mappings", "", "Path to YAML file containing registry mappings")

	return cmd
}

// runDefault implements the original functionality
func runDefault(cmd *cobra.Command, args []string) error {
	// Initialize debug logging
	debug.Init(debugEnabled)
	debug.FunctionEnter("runDefault")
	defer debug.FunctionExit("runDefault")

	// Validate required flags
	if chartPath == "" {
		return fmt.Errorf("--chart-path is required")
	}

	if targetRegistry == "" {
		return fmt.Errorf("--target-registry is required")
	}

	if sourceRegistries == "" {
		return fmt.Errorf("--source-registries is required")
	}

	// Parse the comma-separated lists into slices
	sourceRegistriesList := strings.Split(sourceRegistries, ",")
	var excludeRegistriesList []string
	if excludeRegistries != "" {
		excludeRegistriesList = strings.Split(excludeRegistries, ",")
	}

	debug.DumpValue("Source Registries", sourceRegistriesList)
	debug.DumpValue("Exclude Registries", excludeRegistriesList)

	// Validate path strategy
	if pathStrategy != "prefix-source-registry" {
		return fmt.Errorf("unsupported path strategy: %s (currently only 'prefix-source-registry' is supported)", pathStrategy)
	}

	// Load the chart
	if verbose {
		fmt.Printf("Loading chart from: %s\n", chartPath)
	}
	debug.Printf("Loading chart from: %s", chartPath)
	chartData, err := chart.LoadChart(chartPath)
	if err != nil {
		return fmt.Errorf("error loading chart: %v", err)
	}

	// Print values for debugging if verbose
	if verbose {
		fmt.Println("Chart values:")
		fmt.Printf("%+v\n", chartData.Values)
	}
	debug.DumpValue("Chart Values", chartData.Values)

	// Load registry mappings if provided
	var registryMappings *registry.RegistryMappings
	if registryMappingsFile != "" {
		var err error
		registryMappings, err = registry.LoadMappings(registryMappingsFile)
		if err != nil {
			return fmt.Errorf("error loading registry mappings: %v", err)
		}
		if verbose {
			fmt.Printf("Loaded registry mappings from: %s\n", registryMappingsFile)
			for _, m := range registryMappings.Mappings {
				fmt.Printf("  %s -> %s\n", m.Source, m.Target)
			}
		}
	} else if verbose {
		fmt.Println("No registry mappings file provided, using default mapping behavior:")
		fmt.Printf("  docker.io -> %s/dockerio\n", targetRegistry)
		fmt.Printf("  quay.io -> %s/quayio\n", targetRegistry)
		fmt.Printf("  gcr.io -> %s/gcrio\n", targetRegistry)
	}

	// Get the path strategy
	strategy, err := strategy.GetStrategy(pathStrategy, registryMappings)
	if err != nil {
		return fmt.Errorf("error getting path strategy: %v", err)
	}

	// First detect images to see what we find
	if verbose || debugEnabled {
		detectedImages, unsupported, err := image.DetectImages(chartData.Values, []string{}, sourceRegistriesList, excludeRegistriesList, strictMode)
		if err != nil {
			return fmt.Errorf("error detecting images: %v", err)
		} else {
			if verbose {
				fmt.Printf("Detected %d images:\n", len(detectedImages))
				for _, img := range detectedImages {
					fmt.Printf("  Path: %v\n  Type: %v\n  Image: %+v\n", img.Location, img.LocationType, img.Reference)
				}
				if len(unsupported) > 0 {
					fmt.Printf("Found %d unsupported structures:\n", len(unsupported))
					for _, u := range unsupported {
						fmt.Printf("  Path: %v\n  Type: %v\n", u.Location, u.LocationType)
					}
				}
			}
			debug.DumpValue("Detected Images", detectedImages)
			debug.DumpValue("Unsupported Structures", unsupported)
		}
	}

	overrides, err := chart.GenerateOverrides(chartData, targetRegistry, sourceRegistriesList, excludeRegistriesList, strategy, registryMappings, verbose)
	if err != nil {
		return fmt.Errorf("error generating overrides: %v", err)
	}

	debug.DumpValue("Generated Overrides", overrides)

	// Convert overrides to YAML
	yamlData, err := chart.OverridesToYAML(overrides)
	if err != nil {
		return fmt.Errorf("error converting overrides to YAML: %v", err)
	}

	debug.DumpValue("YAML Output", string(yamlData))

	// Validate the generated overrides by attempting a dry-run helm template
	if !dryRun {
		if err := validateHelmTemplate(chartPath, yamlData); err != nil {
			return fmt.Errorf("error validating generated overrides: %v", err)
		}
	}

	// Output the results
	if outputFile == "" || dryRun {
		fmt.Println(string(yamlData))
	}

	if outputFile != "" && !dryRun {
		err = os.WriteFile(outputFile, yamlData, 0644)
		if err != nil {
			return fmt.Errorf("error writing output file: %v", err)
		}
		if verbose {
			fmt.Printf("Wrote overrides to: %s\n", outputFile)
		}
		debug.Printf("Wrote overrides to: %s", outputFile)
	}

	// Exit successfully
	return nil
}

// validateHelmTemplate validates the generated overrides by attempting a helm template.
// @param chartPath: Path to the Helm chart
// @param overrides: Generated YAML overrides
// @returns: error if validation fails
// @llm-helper This function ensures the overrides are valid
// @llm-helper Uses helm template command for validation
// @llm-helper Provides detailed error messages
func validateHelmTemplate(chartPath string, overrides []byte) error {
	// Create a temporary file for the overrides
	tmpFile, err := os.CreateTemp("", "helm-override-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove temporary file %s: %v\n", tmpFile.Name(), err)
		}
	}()

	// Write the overrides to the temporary file
	if err := os.WriteFile(tmpFile.Name(), overrides, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %v", err)
	}

	// Run helm template with the overrides
	cmd := exec.Command("helm", "template", "test", chartPath, "-f", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Parse the error output to provide more helpful messages
		errorMsg := string(output)
		if strings.Contains(errorMsg, "could not find template") {
			return fmt.Errorf("helm template error: missing template file\nDetails: %s", errorMsg)
		} else if strings.Contains(errorMsg, "error converting YAML to JSON") {
			return fmt.Errorf("helm template error: invalid YAML syntax\nDetails: %s", errorMsg)
		} else if strings.Contains(errorMsg, "error validating data") {
			return fmt.Errorf("helm template error: validation failed\nDetails: %s", errorMsg)
		} else {
			return fmt.Errorf("helm template error: %s", errorMsg)
		}
	}

	return nil
}
