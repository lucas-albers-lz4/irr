package rules

import (
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/debug"
	"helm.sh/helm/v3/pkg/chart"
)

// Rule defines the interface for a chart-specific rule
type Rule interface {
	// Name returns the unique name of this rule
	Name() string

	// Description returns a human-readable description of this rule
	Description() string

	// AppliesTo determines if this rule applies to the given chart
	AppliesTo(ch *chart.Chart) (Detection, bool)

	// Parameters returns the parameters that should be set if this rule
	// is applied to a chart
	Parameters() []Parameter

	// Priority returns the rule's priority (higher numbers have higher priority)
	Priority() int
}

// BaseRule provides a base implementation that can be embedded in other rules
type BaseRule struct {
	name        string
	description string
	parameters  []Parameter
	priority    int
}

// NewBaseRule creates a new BaseRule
func NewBaseRule(name, description string, parameters []Parameter, priority int) BaseRule {
	return BaseRule{
		name:        name,
		description: description,
		parameters:  parameters,
		priority:    priority,
	}
}

// Name returns the rule name
func (r BaseRule) Name() string {
	return r.name
}

// Description returns the rule description
func (r BaseRule) Description() string {
	return r.description
}

// Parameters returns the rule parameters
func (r BaseRule) Parameters() []Parameter {
	return r.parameters
}

// Priority returns the rule priority
func (r BaseRule) Priority() int {
	return r.priority
}

// AppliesTo base implementation always returns false - should be overridden
func (r BaseRule) AppliesTo(ch *chart.Chart) (Detection, bool) {
	return Detection{
		Provider:   ProviderUnknown,
		Confidence: ConfidenceNone,
	}, false
}

// ApplyRulesToMap applies the parameters from matching rules to the given override map
// but only includes Type 1 (Deployment-Critical) parameters
func ApplyRulesToMap(rules []Rule, ch *chart.Chart, overrideMap map[string]interface{}) (bool, error) {
	if len(rules) == 0 || ch == nil {
		return false, nil
	}

	debug.Printf("Checking %d rules for chart: %s", len(rules), ch.Name())

	appliedAny := false
	for _, rule := range rules {
		detection, applies := rule.AppliesTo(ch)
		if !applies {
			continue
		}

		debug.Printf("Rule '%s' applies to chart '%s' (confidence: %d)",
			rule.Name(), ch.Name(), detection.Confidence)

		// Apply all Type 1 (Deployment-Critical) parameters to the override map
		for _, param := range rule.Parameters() {
			if param.Type == TypeDeploymentCritical {
				// Split the path by dots and set the value in the nested map
				if err := setValueAtPath(overrideMap, param.Path, param.Value); err != nil {
					return appliedAny, fmt.Errorf("failed to set parameter %s: %w", param.Path, err)
				}

				debug.Printf("Applied parameter '%s' = '%v' to chart '%s'",
					param.Path, param.Value, ch.Name())

				appliedAny = true
			}
		}
	}

	return appliedAny, nil
}

// setValueAtPath sets a value at the specified dot-notation path in a nested map
func setValueAtPath(m map[string]interface{}, path string, value interface{}) error {
	// For now, use a simple implementation. In a real implementation,
	// this would use a more robust method or reuse code from elsewhere in the codebase.

	// We can enhance this later with a proper implementation that handles arrays,
	// creates nested maps as needed, etc.

	// This is just a placeholder implementation to demonstrate the concept
	parts := splitPath(path)
	return setValueInMap(m, parts, value)
}

// splitPath splits a dot-notation path into parts
func splitPath(path string) []string {
	// This is a simplified implementation
	// In a real implementation, this would handle escaping, arrays, etc.
	return strings.Split(path, ".")
}

// setValueInMap sets a value in a nested map based on path parts
func setValueInMap(m map[string]interface{}, parts []string, value interface{}) error {
	if len(parts) == 0 {
		return fmt.Errorf("empty path")
	}

	if len(parts) == 1 {
		// We've reached the final part, set the value
		m[parts[0]] = value
		return nil
	}

	// We need to traverse deeper
	key := parts[0]
	rest := parts[1:]

	// If the key doesn't exist or isn't a map, create a new map
	if m[key] == nil {
		m[key] = make(map[string]interface{})
	}

	// Convert to map[string]interface{} if possible
	subMap, ok := m[key].(map[string]interface{})
	if !ok {
		// Key exists but isn't a map
		return fmt.Errorf("path element %q is not a map", key)
	}

	// Recursively set in the sub-map
	return setValueInMap(subMap, rest, value)
}

func (r *BaseRule) SetChart(_ *chart.Chart) {
	// Implementation of SetChart method
}
