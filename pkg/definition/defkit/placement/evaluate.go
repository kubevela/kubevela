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

import (
	"fmt"
	"strings"
)

// Evaluate checks if the given cluster labels satisfy the placement constraints.
// Returns a PlacementResult with eligibility status and explanation.
//
// Evaluation logic:
//   - If no constraints are defined, the definition is eligible (runs everywhere)
//   - If RunOn is specified, all RunOn conditions must match
//   - If NotRunOn is specified, none of the NotRunOn conditions must match
//   - Final eligibility = (matches RunOn OR RunOn is empty) AND (does not match NotRunOn)
func Evaluate(spec PlacementSpec, labels map[string]string) PlacementResult {
	if labels == nil {
		labels = map[string]string{}
	}

	// No constraints means eligible everywhere
	if spec.IsEmpty() {
		return PlacementResult{
			Eligible: true,
			Reason:   "no placement constraints defined",
		}
	}

	result := PlacementResult{
		Eligible: true,
	}

	// Check RunOn conditions (must all match if specified)
	if len(spec.RunOn) > 0 {
		allRunOnMatch := true
		var matchedRunOn []string
		var failedRunOn []string

		for _, cond := range spec.RunOn {
			if cond.Evaluate(labels) {
				matchedRunOn = append(matchedRunOn, cond.String())
			} else {
				allRunOnMatch = false
				failedRunOn = append(failedRunOn, cond.String())
			}
		}

		result.MatchedRunOn = matchedRunOn

		if !allRunOnMatch {
			result.Eligible = false
			result.Reason = fmt.Sprintf("runOn conditions not satisfied: %s", strings.Join(failedRunOn, ", "))
			return result
		}
	}

	// Check NotRunOn conditions (none must match)
	if len(spec.NotRunOn) > 0 {
		for _, cond := range spec.NotRunOn {
			if cond.Evaluate(labels) {
				result.MatchedNotRunOn = append(result.MatchedNotRunOn, cond.String())
				result.Eligible = false
				result.Reason = fmt.Sprintf("excluded by notRunOn: %s", cond.String())
				return result
			}
		}
	}

	// Build success reason
	if len(result.MatchedRunOn) > 0 {
		result.Reason = fmt.Sprintf("runOn conditions satisfied: %s", strings.Join(result.MatchedRunOn, ", "))
	} else {
		result.Reason = "no runOn constraints, not excluded by notRunOn"
	}

	return result
}

// Evaluate for LabelCondition checks if the label matches the condition.
func (c *LabelCondition) Evaluate(labels map[string]string) bool {
	value, exists := labels[c.Key]

	switch c.Operator {
	case OperatorEquals:
		if len(c.Values) == 0 {
			return false
		}
		return exists && value == c.Values[0]

	case OperatorNotEquals:
		if len(c.Values) == 0 {
			// Fail closed: empty values is an invalid constraint, match nothing
			// This is consistent with Kubernetes which requires non-empty values
			// for In/NotIn operators, and matches the behavior of OperatorEquals
			return false
		}
		return !exists || value != c.Values[0]

	case OperatorIn:
		if !exists {
			return false
		}
		for _, v := range c.Values {
			if value == v {
				return true
			}
		}
		return false

	case OperatorNotIn:
		if len(c.Values) == 0 {
			return false
		}
		if !exists {
			return true
		}
		for _, v := range c.Values {
			if value == v {
				return false
			}
		}
		return true

	case OperatorExists:
		return exists

	case OperatorNotExists:
		return !exists

	default:
		return false
	}
}

// String returns a human-readable representation of the LabelCondition.
func (c *LabelCondition) String() string {
	switch c.Operator {
	case OperatorEquals:
		if len(c.Values) > 0 {
			return fmt.Sprintf("%s = %s", c.Key, c.Values[0])
		}
		return fmt.Sprintf("%s = <empty>", c.Key)

	case OperatorNotEquals:
		if len(c.Values) > 0 {
			return fmt.Sprintf("%s != %s", c.Key, c.Values[0])
		}
		return fmt.Sprintf("%s != <empty>", c.Key)

	case OperatorIn:
		return fmt.Sprintf("%s in (%s)", c.Key, strings.Join(c.Values, ", "))

	case OperatorNotIn:
		return fmt.Sprintf("%s not in (%s)", c.Key, strings.Join(c.Values, ", "))

	case OperatorExists:
		return fmt.Sprintf("%s exists", c.Key)

	case OperatorNotExists:
		return fmt.Sprintf("%s not exists", c.Key)

	default:
		return fmt.Sprintf("%s %s %v", c.Key, c.Operator, c.Values)
	}
}

// Evaluate for AllCondition returns true if all conditions match.
func (c *AllCondition) Evaluate(labels map[string]string) bool {
	if len(c.Conditions) == 0 {
		return true
	}
	for _, cond := range c.Conditions {
		if !cond.Evaluate(labels) {
			return false
		}
	}
	return true
}

// String returns a human-readable representation of the AllCondition.
func (c *AllCondition) String() string {
	if len(c.Conditions) == 0 {
		return "all()"
	}
	parts := make([]string, len(c.Conditions))
	for i, cond := range c.Conditions {
		parts[i] = cond.String()
	}
	return fmt.Sprintf("all(%s)", strings.Join(parts, " AND "))
}

// Evaluate for AnyCondition returns true if any condition matches.
func (c *AnyCondition) Evaluate(labels map[string]string) bool {
	if len(c.Conditions) == 0 {
		return false
	}
	for _, cond := range c.Conditions {
		if cond.Evaluate(labels) {
			return true
		}
	}
	return false
}

// String returns a human-readable representation of the AnyCondition.
func (c *AnyCondition) String() string {
	if len(c.Conditions) == 0 {
		return "any()"
	}
	parts := make([]string, len(c.Conditions))
	for i, cond := range c.Conditions {
		parts[i] = cond.String()
	}
	return fmt.Sprintf("any(%s)", strings.Join(parts, " OR "))
}

// Evaluate for NotCondition returns true if the inner condition does not match.
func (c *NotCondition) Evaluate(labels map[string]string) bool {
	if c.Condition == nil {
		return true
	}
	return !c.Condition.Evaluate(labels)
}

// String returns a human-readable representation of the NotCondition.
func (c *NotCondition) String() string {
	if c.Condition == nil {
		return "not(<nil>)"
	}
	return fmt.Sprintf("not(%s)", c.Condition.String())
}

// Helper functions for creating conditions

// Label creates a new LabelCondition builder for the given key.
func Label(key string) *LabelConditionBuilder {
	return &LabelConditionBuilder{key: key}
}

// LabelConditionBuilder provides a fluent API for creating LabelConditions.
type LabelConditionBuilder struct {
	key string
}

// Eq creates a condition that matches when the label equals the value.
func (b *LabelConditionBuilder) Eq(value string) *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorEquals,
		Values:   []string{value},
	}
}

// Ne creates a condition that matches when the label does not equal the value.
func (b *LabelConditionBuilder) Ne(value string) *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorNotEquals,
		Values:   []string{value},
	}
}

// In creates a condition that matches when the label value is in the set.
func (b *LabelConditionBuilder) In(values ...string) *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorIn,
		Values:   values,
	}
}

// NotIn creates a condition that matches when the label value is not in the set.
func (b *LabelConditionBuilder) NotIn(values ...string) *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorNotIn,
		Values:   values,
	}
}

// Exists creates a condition that matches when the label key exists.
func (b *LabelConditionBuilder) Exists() *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorExists,
	}
}

// NotExists creates a condition that matches when the label key does not exist.
func (b *LabelConditionBuilder) NotExists() *LabelCondition {
	return &LabelCondition{
		Key:      b.key,
		Operator: OperatorNotExists,
	}
}

// All creates an AllCondition that requires all conditions to match.
func All(conditions ...Condition) *AllCondition {
	return &AllCondition{Conditions: conditions}
}

// Any creates an AnyCondition that requires at least one condition to match.
func Any(conditions ...Condition) *AnyCondition {
	return &AnyCondition{Conditions: conditions}
}

// Not creates a NotCondition that negates the given condition.
func Not(condition Condition) *NotCondition {
	return &NotCondition{Condition: condition}
}
