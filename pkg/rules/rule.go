package rules

import (
	"fmt"
	"strings"

	"github.com/lucas-albers-lz4/irr/pkg/log"
	"github.com/lucas-albers-lz4/irr/pkg/override"
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

				// NOTE: We need to pass the path parts to override.SetValueAtPath.
				// Since override doesn't export its robust SplitPathWithEscapes, we use the simpler
				// internal one for now, acknowledging its limitations.
				// TODO: Refactor override package to export SplitPathWithEscapes or use a shared path utility.
				pathParts := ParsePath(param.Path) // Using local ParsePath for now

				// Set the value using the override package's utility
				if err := override.SetValueAtPath(overrideMap, pathParts, param.Value); err != nil {
					return appliedAny, fmt.Errorf("failed to set parameter %s for rule %s: %w", param.Path, rule.Name(), err)
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

// ParsePath splits a dot-notation path into parts (basic implementation)
// TODO: Replace with a more robust implementation, possibly from override package if exported.
func ParsePath(path string) []string {
	return strings.Split(path, ".")
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
