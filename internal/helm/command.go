package helm

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

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

// executeHelmCommand executes a helm command with the given arguments
func executeHelmCommand(args []string) (*CommandResult, error) {
	log.Infof("Executing: helm %s", strings.Join(args, " "))

	// #nosec G204 -- We need to allow variable arguments to helm command
	cmd := exec.Command("helm", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute helm command
	err := cmd.Run()

	result := &CommandResult{
		Success: err == nil,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Error:   err,
	}

	// Check if command succeeded
	if err != nil {
		log.Errorf("Helm command failed: %v", err)
		log.Errorf("Stderr: %s", stderr.String())
		return result, fmt.Errorf("helm command failed: %w\n%s", err, stderr.String())
	}

	return result, nil
}
