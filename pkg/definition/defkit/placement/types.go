/*
Copyright 2025 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package placement

const (
	// ClusterIdentityConfigMapName is the well-known name of the ConfigMap
	// that stores cluster identity labels for definition placement.
	ClusterIdentityConfigMapName = "vela-cluster-identity"

	// ClusterIdentityNamespace is the namespace where the cluster identity
	// ConfigMap is expected to be found.
	ClusterIdentityNamespace = "vela-system"
)

// Operator represents a comparison operator for label matching.
type Operator string

const (
	// OperatorEquals matches when the label value equals the specified value.
	OperatorEquals Operator = "Eq"
	// OperatorNotEquals matches when the label value does not equal the specified value.
	OperatorNotEquals Operator = "Ne"
	// OperatorIn matches when the label value is in the specified set of values.
	OperatorIn Operator = "In"
	// OperatorNotIn matches when the label value is not in the specified set of values.
	OperatorNotIn Operator = "NotIn"
	// OperatorExists matches when the label key exists (regardless of value).
	OperatorExists Operator = "Exists"
	// OperatorNotExists matches when the label key does not exist.
	OperatorNotExists Operator = "NotExists"
)

// Condition represents a placement condition that can be evaluated
// against a set of cluster labels.
type Condition interface {
	// Evaluate returns true if the condition matches the given labels.
	Evaluate(labels map[string]string) bool
	// String returns a human-readable representation of the condition.
	String() string
}

// LabelCondition represents a condition on a single cluster label.
type LabelCondition struct {
	// Key is the label key to match against.
	Key string `json:"key" yaml:"key"`
	// Operator is the comparison operator.
	Operator Operator `json:"operator" yaml:"operator"`
	// Values are the values to compare against (used by Eq, Ne, In, NotIn).
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
}

// AllCondition represents a logical AND of multiple conditions.
// All conditions must match for the AllCondition to match.
type AllCondition struct {
	// Conditions are the conditions to AND together.
	Conditions []Condition `json:"conditions" yaml:"conditions"`
}

// AnyCondition represents a logical OR of multiple conditions.
// At least one condition must match for the AnyCondition to match.
type AnyCondition struct {
	// Conditions are the conditions to OR together.
	Conditions []Condition `json:"conditions" yaml:"conditions"`
}

// NotCondition represents a logical NOT of a condition.
// Matches when the inner condition does not match.
type NotCondition struct {
	// Condition is the condition to negate.
	Condition Condition `json:"condition" yaml:"condition"`
}

// PlacementSpec defines where a definition can and cannot run.
type PlacementSpec struct {
	// RunOn specifies conditions that must be satisfied for the definition
	// to be applied. If multiple conditions are provided, they are ANDed together.
	// If empty, the definition can run on any cluster.
	RunOn []Condition `json:"runOn,omitempty" yaml:"runOn,omitempty"`
	// NotRunOn specifies conditions that exclude clusters from running the
	// definition. If any condition matches, the definition will not be applied.
	// If empty, no clusters are excluded.
	NotRunOn []Condition `json:"notRunOn,omitempty" yaml:"notRunOn,omitempty"`
}

// PlacementResult represents the result of evaluating placement constraints.
type PlacementResult struct {
	// Eligible indicates whether the definition can run on the cluster.
	Eligible bool
	// Reason provides a human-readable explanation for the result.
	Reason string
	// MatchedRunOn indicates which RunOn conditions matched (if any).
	MatchedRunOn []string
	// MatchedNotRunOn indicates which NotRunOn conditions matched (if any).
	MatchedNotRunOn []string
}

// IsEmpty returns true if no placement constraints are defined.
func (p *PlacementSpec) IsEmpty() bool {
	return len(p.RunOn) == 0 && len(p.NotRunOn) == 0
}

// GetEffectivePlacement returns the effective placement by combining module-level
// and definition-level placements. If the definition has placement constraints,
// they override the module defaults. If the definition has no placement constraints,
// the module defaults are used.
func GetEffectivePlacement(modulePlacement, definitionPlacement PlacementSpec) PlacementSpec {
	// If definition has placement, it overrides module defaults
	if !definitionPlacement.IsEmpty() {
		return definitionPlacement
	}
	// Otherwise use module defaults
	return modulePlacement
}
