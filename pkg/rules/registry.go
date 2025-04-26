package rules

import (
	"sort"
	"sync"

	"github.com/lucas-albers-lz4/irr/pkg/log"
	"helm.sh/helm/v3/pkg/chart"
)

// Registry manages the collection of rules and provides methods
// to add, get, and apply rules to charts
type Registry struct {
	rules   []Rule
	enabled bool
	mu      sync.RWMutex
}

// NewRegistry creates a new rule registry with default rules
func NewRegistry() *Registry {
	registry := &Registry{
		rules:   []Rule{},
		enabled: true,
	}

	// Register default rules
	registry.AddRule(NewBitnamiSecurityBypassRule())

	log.Debug("Created rule registry with %d default rules", len(registry.rules))
	return registry
}

// AddRule adds a rule to the registry
func (r *Registry) AddRule(rule Rule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = append(r.rules, rule)

	// Sort rules by priority (higher first)
	sort.Slice(r.rules, func(i, j int) bool {
		return r.rules[i].Priority() > r.rules[j].Priority()
	})

	log.Debug("Added rule '%s' to registry", rule.Name())
}

// GetRules returns all registered rules
func (r *Registry) GetRules() []Rule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent race conditions
	result := make([]Rule, len(r.rules))
	copy(result, r.rules)
	return result
}

// IsEnabled returns whether the rules system is enabled
func (r *Registry) IsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enabled
}

// SetEnabled enables or disables the rules system
func (r *Registry) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enabled = enabled
	log.Debug("Rules system enabled: %v", enabled)
}

// ApplyRules applies all matching rules to the chart's override map
// but only includes Type 1 (Deployment-Critical) parameters
func (r *Registry) ApplyRules(ch *chart.Chart, overrideMap map[string]interface{}) (bool, error) {
	if !r.IsEnabled() {
		log.Debug("Rules system is disabled, skipping rule application")
		return false, nil
	}

	rules := r.GetRules()
	return ApplyRulesToMap(rules, ch, overrideMap)
}

// DefaultRegistry is the global default rule registry
var DefaultRegistry = NewRegistry()
