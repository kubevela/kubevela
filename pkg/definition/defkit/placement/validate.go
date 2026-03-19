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

// ValidationError represents a placement validation error.
type ValidationError struct {
	Message  string
	RunOn    string
	NotRunOn string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// ValidatePlacement checks for conflicting placement constraints.
// Returns an error if the constraints would prevent the definition from
// running anywhere.
//
// Conflict detection covers:
//   - Identical conditions in RunOn and NotRunOn
//   - Label conditions that logically conflict (e.g., Eq vs Exists on same key)
//   - Composite conditions (All, Any, Not) with conflicting inner conditions
//   - Deeply nested conditions
func ValidatePlacement(spec PlacementSpec) error {
	if spec.IsEmpty() {
		return nil
	}

	// Check for conflicts between RunOn and NotRunOn conditions
	for _, runCond := range spec.RunOn {
		for _, notRunCond := range spec.NotRunOn {
			if conflict, reason := conditionsConflict(runCond, notRunCond); conflict {
				return &ValidationError{
					Message:  fmt.Sprintf("conflicting placement constraints: %s - definition may never be eligible to run", reason),
					RunOn:    runCond.String(),
					NotRunOn: notRunCond.String(),
				}
			}
		}
	}

	return nil
}

// conditionsConflict checks if two conditions conflict with each other.
// Returns true and a reason if the conditions conflict.
//
// Two conditions conflict if satisfying the RunOn condition would always
// trigger the NotRunOn condition, making the definition ineligible.
func conditionsConflict(runOn, notRunOn Condition) (bool, string) {
	// Check for exact match (same condition in both)
	if runOn.String() == notRunOn.String() {
		return true, fmt.Sprintf("identical condition '%s' in both RunOn and NotRunOn", runOn.String())
	}

	// Extract required conditions from RunOn (what MUST be true)
	runOnRequired := extractRequiredConditions(runOn)

	// Extract conditions from NotRunOn that would exclude (what must NOT match)
	notRunOnExcluders := extractExcludingConditions(notRunOn)

	// Check if any required condition is excluded
	for _, req := range runOnRequired {
		for _, excl := range notRunOnExcluders {
			if conflict, reason := leafConditionsConflict(req, excl); conflict {
				return true, reason
			}
		}
	}

	// Additional checks for composite condition patterns
	if conflict, reason := checkCompositePatterns(runOn, notRunOn); conflict {
		return true, reason
	}

	return false, ""
}

// extractRequiredConditions extracts leaf conditions that MUST be satisfied.
// For All(A, B): both A and B are required
// For Any(A, B): neither is individually required (returns empty - handled separately)
// For Not(A): the negation is required (returns the Not wrapper)
// For Label: the label condition itself is required
func extractRequiredConditions(cond Condition) []Condition {
	switch c := cond.(type) {
	case *LabelCondition:
		return []Condition{c}

	case *AllCondition:
		// All conditions are required
		var required []Condition
		for _, inner := range c.Conditions {
			required = append(required, extractRequiredConditions(inner)...)
		}
		return required

	case *AnyCondition:
		// None are individually required, but we need to track the Any itself
		// for pattern matching
		return []Condition{c}

	case *NotCondition:
		// The Not condition itself is required
		return []Condition{c}

	default:
		return nil
	}
}

// extractExcludingConditions extracts conditions that would cause exclusion.
// For Any(A, B): either A or B matching causes exclusion
// For All(A, B): both must match to cause exclusion (returns the All wrapper)
// For Not(A): A not matching causes exclusion
// For Label: the label condition itself causes exclusion
func extractExcludingConditions(cond Condition) []Condition {
	switch c := cond.(type) {
	case *LabelCondition:
		return []Condition{c}

	case *AnyCondition:
		// Any matching condition causes exclusion
		var excluders []Condition
		for _, inner := range c.Conditions {
			excluders = append(excluders, extractExcludingConditions(inner)...)
		}
		return excluders

	case *AllCondition:
		// All must match to cause exclusion - return the All itself
		return []Condition{c}

	case *NotCondition:
		// The Not condition itself causes exclusion
		return []Condition{c}

	default:
		return nil
	}
}

// leafConditionsConflict checks if two leaf conditions conflict.
func leafConditionsConflict(required, excluder Condition) (bool, string) {
	// Handle label conditions
	reqLabel, reqIsLabel := required.(*LabelCondition)
	exclLabel, exclIsLabel := excluder.(*LabelCondition)

	if reqIsLabel && exclIsLabel {
		return labelConditionsConflict(reqLabel, exclLabel)
	}

	// Handle Any in required vs label excluder
	if reqAny, isAny := required.(*AnyCondition); isAny {
		if exclIsLabel {
			// Check if ALL options in Any are excluded
			allExcluded := true
			for _, inner := range reqAny.Conditions {
				if innerLabel, ok := inner.(*LabelCondition); ok {
					if conflict, _ := labelConditionsConflict(innerLabel, exclLabel); !conflict {
						allExcluded = false
						break
					}
				} else {
					allExcluded = false
					break
				}
			}
			if allExcluded && len(reqAny.Conditions) > 0 {
				return true, fmt.Sprintf("all options in '%s' are excluded by '%s'", reqAny.String(), exclLabel.String())
			}
		}
	}

	// Handle All in excluder
	if exclAll, isAll := excluder.(*AllCondition); isAll {
		if reqIsLabel {
			// Check if required condition is part of the All
			for _, inner := range exclAll.Conditions {
				if innerLabel, ok := inner.(*LabelCondition); ok {
					if conflict, reason := labelConditionsConflict(reqLabel, innerLabel); conflict {
						// The All won't necessarily match just because one part matches
						// So this isn't a definite conflict unless All has only one condition
						if len(exclAll.Conditions) == 1 {
							return true, reason
						}
					}
				}
			}
		}
	}

	return false, ""
}

// labelConditionsConflict checks if two label conditions on the same key conflict.
func labelConditionsConflict(runOn, notRunOn *LabelCondition) (bool, string) {
	// Only check conditions on the same key
	if runOn.Key != notRunOn.Key {
		return false, ""
	}

	key := runOn.Key

	// Case 1: RunOn requires Eq/In, NotRunOn has Exists
	// e.g., RunOn(cloud=aws), NotRunOn(cloud exists) -> always excluded when condition is met
	if (runOn.Operator == OperatorEquals || runOn.Operator == OperatorIn) &&
		notRunOn.Operator == OperatorExists {
		return true, fmt.Sprintf("RunOn requires '%s' to have a specific value, but NotRunOn excludes when '%s' exists", key, key)
	}

	// Case 2: RunOn requires Exists, NotRunOn also has Exists
	if runOn.Operator == OperatorExists && notRunOn.Operator == OperatorExists {
		return true, fmt.Sprintf("'%s exists' is in both RunOn and NotRunOn", key)
	}

	// Case 3: RunOn Eq value X, NotRunOn Eq same value X
	if runOn.Operator == OperatorEquals && notRunOn.Operator == OperatorEquals {
		if len(runOn.Values) > 0 && len(notRunOn.Values) > 0 &&
			runOn.Values[0] == notRunOn.Values[0] {
			return true, fmt.Sprintf("'%s = %s' is required by RunOn but excluded by NotRunOn", key, runOn.Values[0])
		}
	}

	// Case 4: RunOn In values, NotRunOn In overlapping values
	if runOn.Operator == OperatorIn && notRunOn.Operator == OperatorIn {
		overlap := findOverlap(runOn.Values, notRunOn.Values)
		if len(overlap) == len(runOn.Values) {
			// All RunOn values are excluded by NotRunOn
			return true, fmt.Sprintf("all values for '%s' required by RunOn (%s) are excluded by NotRunOn", key, strings.Join(runOn.Values, ", "))
		}
	}

	// Case 5: RunOn Eq value, NotRunOn In contains that value
	if runOn.Operator == OperatorEquals && notRunOn.Operator == OperatorIn {
		if len(runOn.Values) > 0 && contains(notRunOn.Values, runOn.Values[0]) {
			return true, fmt.Sprintf("'%s = %s' is required by RunOn but '%s' is in NotRunOn exclusion list", key, runOn.Values[0], runOn.Values[0])
		}
	}

	// Case 6: RunOn In values, NotRunOn Eq one of those values - partial conflict
	if runOn.Operator == OperatorIn && notRunOn.Operator == OperatorEquals {
		if len(notRunOn.Values) > 0 && contains(runOn.Values, notRunOn.Values[0]) {
			// Only flag if all values would be excluded
			if len(runOn.Values) == 1 {
				return true, fmt.Sprintf("the only allowed value '%s' for '%s' is excluded by NotRunOn", notRunOn.Values[0], key)
			}
		}
	}

	// Case 7: RunOn Exists, NotRunOn NotExists - contradiction
	if runOn.Operator == OperatorExists && notRunOn.Operator == OperatorNotExists {
		return true, fmt.Sprintf("'%s' must exist (RunOn) but must not exist (NotRunOn)", key)
	}

	// Case 8: RunOn NotExists, NotRunOn Exists - not a direct conflict
	// (different states, but could be problematic in combination with other conditions)

	return false, ""
}

// checkCompositePatterns checks for specific composite condition patterns that conflict.
func checkCompositePatterns(runOn, notRunOn Condition) (bool, string) {
	// Pattern: RunOn(Any(A, B)) + NotRunOn(Any(A, B)) - identical Any conditions
	if runAny, isRunAny := runOn.(*AnyCondition); isRunAny {
		if notRunAny, isNotRunAny := notRunOn.(*AnyCondition); isNotRunAny {
			if anyConditionsOverlap(runAny, notRunAny) {
				return true, fmt.Sprintf("Any conditions overlap: RunOn(%s) conflicts with NotRunOn(%s)", runAny.String(), notRunAny.String())
			}
		}
	}

	// Pattern: RunOn(All(A, B)) + NotRunOn(Any(A, C)) - A is required and excluded
	if runAll, isRunAll := runOn.(*AllCondition); isRunAll {
		if notRunAny, isNotRunAny := notRunOn.(*AnyCondition); isNotRunAny {
			for _, runInner := range runAll.Conditions {
				for _, notRunInner := range notRunAny.Conditions {
					if runInner.String() == notRunInner.String() {
						return true, fmt.Sprintf("'%s' is required by All() but excluded by Any() in NotRunOn", runInner.String())
					}
					// Also check for label conflicts
					if runLabel, ok := runInner.(*LabelCondition); ok {
						if notRunLabel, ok := notRunInner.(*LabelCondition); ok {
							if conflict, reason := labelConditionsConflict(runLabel, notRunLabel); conflict {
								return true, fmt.Sprintf("condition in All() conflicts with Any() in NotRunOn: %s", reason)
							}
						}
					}
				}
			}
		}
	}

	// Pattern: RunOn(A) + NotRunOn(All(A, B)) - not a conflict (B might not match)
	// Pattern: RunOn(Any(A, B)) + NotRunOn(All(A, B)) - not a conflict (Any needs one, All needs both)

	// Pattern: Nested Any - RunOn(Any(Any(A, B), C)) should flatten
	// This is handled by the recursive extraction

	return false, ""
}

// anyConditionsOverlap checks if two Any conditions have overlapping leaf conditions.
func anyConditionsOverlap(a, b *AnyCondition) bool {
	aLeaves := extractAllLeafConditions(a)
	bLeaves := extractAllLeafConditions(b)

	// Check if all leaves in a have a matching leaf in b
	matchCount := 0
	for _, aLeaf := range aLeaves {
		for _, bLeaf := range bLeaves {
			if aLeaf.String() == bLeaf.String() {
				matchCount++
				break
			}
			// Also check label conflicts (aLeaf and bLeaf are already *LabelCondition)
			if conflict, _ := labelConditionsConflict(aLeaf, bLeaf); conflict {
				matchCount++
				break
			}
		}
	}

	// If all RunOn options are excluded by NotRunOn, it's a conflict
	return matchCount == len(aLeaves) && len(aLeaves) > 0
}

// extractAllLeafConditions recursively extracts all leaf LabelConditions.
func extractAllLeafConditions(cond Condition) []*LabelCondition {
	var leaves []*LabelCondition

	switch c := cond.(type) {
	case *LabelCondition:
		leaves = append(leaves, c)

	case *AllCondition:
		for _, inner := range c.Conditions {
			leaves = append(leaves, extractAllLeafConditions(inner)...)
		}

	case *AnyCondition:
		for _, inner := range c.Conditions {
			leaves = append(leaves, extractAllLeafConditions(inner)...)
		}

	case *NotCondition:
		if c.Condition != nil {
			leaves = append(leaves, extractAllLeafConditions(c.Condition)...)
		}
	}

	return leaves
}

// findOverlap returns values that exist in both slices.
func findOverlap(a, b []string) []string {
	var overlap []string
	for _, v := range a {
		if contains(b, v) {
			overlap = append(overlap, v)
		}
	}
	return overlap
}

// contains checks if a slice contains a value.
func contains(slice []string, value string) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
