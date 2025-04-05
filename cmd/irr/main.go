package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// newExitCodeError creates an error associated with an exit code
// Note: This requires a custom error type or wrapping logic to properly handle exit codes.
// For simplicity, just wrapping standard errors for now.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	return e.Err.Error()
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}

func newExitCodeError(code int, msg string) error {
	// Create the base error message first using errors.New
	baseErr := errors.New(msg)
	return &ExitCodeError{Code: code, Err: baseErr}
}

// Helper function to wrap existing errors with an exit code
func wrapExitCodeError(code int, baseMsg string, originalErr error) error {
	// Format the combined message safely
	combinedMsg := fmt.Sprintf("%s: %s", baseMsg, originalErr.Error())
	// Wrap the formatted message string using errors.New
	return &ExitCodeError{Code: code, Err: errors.New(combinedMsg)}
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
	// Use the new Loader interface
	loader := chart.NewLoader()
	chartData, err := loader.Load(chartPath)
	if err != nil {
		// Return chart parsing error exit code using wrapper
		return wrapExitCodeError(ExitChartParsingError, "error loading chart", err)
	}

	// Print values for debugging if verbose
	if verbose {
		fmt.Println("Chart values:")
		// Use chartData directly now
		fmt.Printf("%+v\n", chartData.Values)
	}
	debug.DumpValue("Chart Values", chartData.Values)

	// Load registry mappings if provided
	var registryMappings *registry.RegistryMappings
	if registryMappingsFile != "" {
		var mapErr error // Renamed to avoid shadowing
		registryMappings, mapErr = registry.LoadMappings(registryMappingsFile)
		if mapErr != nil {
			return wrapExitCodeError(ExitInputConfigurationError, "error loading registry mappings", mapErr)
		}
		if verbose {
			fmt.Printf("Loaded registry mappings from: %s\n", registryMappingsFile)
			for _, m := range registryMappings.Mappings {
				fmt.Printf("  %s -> %s\n", m.Source, m.Target)
			}
		}
	} else if verbose {
		fmt.Println("No registry mappings file provided, using default mapping behavior:")
		// Example mapping info (actual logic is in strategy)
		// fmt.Printf("  docker.io -> %s/dockerio\n", targetRegistry)
		// fmt.Printf("  quay.io -> %s/quayio\n", targetRegistry)
		// fmt.Printf("  gcr.io -> %s/gcrio\n", targetRegistry)
	}

	// Get the path strategy
	strategyImpl, strategyErr := strategy.GetStrategy(pathStrategy, registryMappings)
	if strategyErr != nil {
		return wrapExitCodeError(ExitCodeInvalidStrategy, "error getting path strategy", strategyErr)
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

	// Create the generator
	generator := chart.NewGenerator(
		chartPath, // Use original path for generator context
		targetRegistry,
		sourceRegistriesList,
		excludeRegistriesList,
		strategyImpl,
		registryMappings,
		strictMode,
		threshold,
		loader, // Pass the loader we created earlier
	)

	// Generate the override file content
	overrideFileResult, generateErr := generator.Generate()
	if generateErr != nil {
		// Check for specific error types from Generate if needed
		if strictMode && strings.Contains(generateErr.Error(), "unsupported structures") {
			return wrapExitCodeError(ExitUnsupportedStructError, "error generating overrides (unsupported structure)", generateErr)
		}
		return wrapExitCodeError(ExitImageProcessingError, "error generating overrides", generateErr)
	}

	debug.DumpValue("Generated Overrides Map", overrideFileResult.Overrides)
	debug.DumpValue("Unsupported Structures Found", overrideFileResult.Unsupported)

	// Convert the resulting overrides map to YAML
	yamlData, yamlErr := chart.OverridesToYAML(overrideFileResult.Overrides)
	if yamlErr != nil {
		return wrapExitCodeError(ExitGeneralRuntimeError, "error converting overrides to YAML", yamlErr)
	}

	debug.DumpValue("YAML Output", string(yamlData))

	// Validate the generated overrides by attempting a dry-run helm template
	if !dryRun {
		// Create the real command runner
		cmdRunner := chart.NewOSCommandRunner() // Assuming constructor exists
		if validationErr := chart.ValidateHelmTemplate(cmdRunner, chartPath, yamlData); validationErr != nil {
			// Wrap error with specific exit code
			return wrapExitCodeError(ExitHelmTemplateError, "Helm template validation failed", validationErr)
		}
	}

	// Write the YAML data to file or stdout
	if !dryRun {
		if outputFile != "" {
			// Ensure the output directory exists
			dir := filepath.Dir(outputFile)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if mkDirErr := os.MkdirAll(dir, 0755); mkDirErr != nil {
					return wrapExitCodeError(ExitInputConfigurationError, "failed to create output directory "+dir, mkDirErr)
				}
			}
			// #nosec G306 // Allow configurable file permissions, default 0644 reasonable
			if writeErr := os.WriteFile(outputFile, yamlData, 0644); writeErr != nil {
				return wrapExitCodeError(ExitInputConfigurationError, "failed to write overrides file", writeErr)
			}
			if verbose {
				fmt.Printf("Overrides successfully written to: %s\n", outputFile)
			}
		} else {
			fmt.Println(string(yamlData))
		}
	} else {
		if verbose {
			fmt.Println("Dry run enabled, printing overrides to stdout:")
		}
		fmt.Println(string(yamlData)) // Print YAML for dry run
		if verbose {
			fmt.Println("\nDry run complete. No files were written.")
		}
	}

	// Report unsupported structures if any, but don't fail if not strict mode and threshold met
	if len(overrideFileResult.Unsupported) > 0 {
		fmt.Fprintln(os.Stderr, "\nWarning: Found unsupported image structures:")
		for _, u := range overrideFileResult.Unsupported {
			fmt.Fprintf(os.Stderr, "  - Path: %s, Type: %s\n", strings.Join(u.Path, "."), u.Type)
		}
	}

	debug.Println("Execution successful")
	return nil // ExitSuccess
}
