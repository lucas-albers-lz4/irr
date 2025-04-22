package rules

import (
	"fmt"
	"strings"

	"github.com/lalbers/irr/pkg/log"
	"helm.sh/helm/v3/pkg/chart"
)

// Rule defines the interface for a chart-specific rule
type Rule interface {
	// Name returns the unique name of this rule
	Name() string

	// Description returns a human-readable description of this rule
	Description() string

	// AppliesTo determines if this rule applies to the given chart
	AppliesTo(*chart.Chart) (Detection, bool)

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
func (r BaseRule) AppliesTo(*chart.Chart) (Detection, bool) {
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

	log.Debug("Checking rules for chart", "rule_count", len(rules), "chart_name", ch.Name())

	appliedAny := false
	for _, rule := range rules {
		detection, applies := rule.AppliesTo(ch)
		if !applies {
			continue
		}

		log.Debug("Rule applies to chart",
			"rule_name", rule.Name(),
			"chart_name", ch.Name(),
			"confidence", detection.Confidence)

		// Apply all Type 1 (Deployment-Critical) parameters to the override map
		for _, param := range rule.Parameters() {
			log.Debug("Checking parameter", "chart_name", ch.Name(), "rule_name", rule.Name(), "param_path", param.Path, "param_type", param.Type)
			if param.Type == TypeDeploymentCritical {
				log.Debug("Attempting to set critical parameter",
					"chart_name", ch.Name(),
					"rule_name", rule.Name(),
					"param_path", param.Path,
					"param_value", param.Value)
				// Split the path by dots and set the value in the nested map
				if err := setValueAtPath(overrideMap, param.Path, param.Value); err != nil {
					return appliedAny, fmt.Errorf("failed to set parameter %s: %w", param.Path, err)
				}

				log.Debug("Applied parameter to chart",
					"param_path", param.Path,
					"param_value", param.Value,
					"chart_name", ch.Name())

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
	// Add nil check for map
	if m == nil {
		return fmt.Errorf("cannot set value in nil map")
	}

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

// GetPriority returns the rule's priority (higher numbers have higher priority)
func (r BaseRule) GetPriority() int {
	return r.priority
}

// SetChart associates the chart with the rule instance.
// This base implementation is a placeholder. Embedding structs can override this
// method if they need to store or process the associated chart.
func (r *BaseRule) SetChart(_ *chart.Chart) {
	// Default implementation: Do nothing. Embedding structs can override this.
}

// SimpleRule is a basic implementation of the Rule interface using BaseRule.
// ... existing code ...

// RegistryInterface defines the method set for applying rules.
// This allows for mocking in tests.
type RegistryInterface interface {
	ApplyRules(chart *chart.Chart, overrides map[string]interface{}) (bool, error)
}

// Ensure Registry implements RegistryInterface
var _ RegistryInterface = (*Registry)(nil)

// AddRule adds a new rule to the registry.
// ... existing code ...
