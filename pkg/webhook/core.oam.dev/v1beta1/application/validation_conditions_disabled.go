/*
Copyright 2026 The KubeVela Authors.

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

package application

import (
	"fmt"
	"strings"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	wfexpressions "github.com/kubevela/workflow/pkg/expressions"
	"k8s.io/apimachinery/pkg/util/validation/field"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

// conditionsWithoutExpressions refuses every $( ) if, on a step or a sub-step, of
// an Application that does not read expressions. Without an evaluator the
// workflow reads such an if as CUE, which it is not, and skips the step.
func conditionsWithoutExpressions(stepsPath *field.Path, steps []wfv1alpha1.WorkflowStep) field.ErrorList {
	var errs field.ErrorList
	for i, step := range steps {
		if wfexpressions.IsCondition(step.If) {
			errs = append(errs, field.Forbidden(stepsPath.Index(i).Child("if"), conditionsDisabledMessage()))
		}
		for j, sub := range step.SubSteps {
			if wfexpressions.IsCondition(sub.If) {
				errs = append(errs, field.Forbidden(stepsPath.Index(i).Child("subSteps").Index(j).Child("if"),
					conditionsDisabledMessage()))
			}
		}
	}
	return errs
}

// conditionsDisabledMessage says which of the two switches is off.
func conditionsDisabledMessage() string {
	if !utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions) {
		return "a $( ) condition needs expressions, which are not enabled on this cluster; " +
			"an operator enables them with the EnableCelExpressions feature gate"
	}
	return fmt.Sprintf("a $( ) condition needs expressions, and this Application has not opted in to them; "+
		"set the %s annotation to \"true\"", oam.AnnotationCelExpressions)
}

// misplacedConditionMessage refuses an if holding a $( ) that is not the whole
// of it.
const misplacedConditionMessage = "a $( ) condition must be the whole if, one expression: " +
	"write $(a && b), not $(a) && $(b)"

// misplacedConditions refuses every if, on a step or a sub-step, that holds a
// $( ) outside a CUE string without being one $( ) expression. Such an if is not
// a condition, and as CUE it never compiles, so the step would be skipped. The
// workflow exports the same check from the version after the one pinned here.
func misplacedConditions(stepsPath *field.Path, steps []wfv1alpha1.WorkflowStep) field.ErrorList {
	var errs field.ErrorList
	for i, step := range steps {
		if misplacedCondition(step.If) {
			errs = append(errs, field.Invalid(stepsPath.Index(i).Child("if"), step.If, misplacedConditionMessage))
		}
		for j, sub := range step.SubSteps {
			if misplacedCondition(sub.If) {
				errs = append(errs, field.Invalid(stepsPath.Index(i).Child("subSteps").Index(j).Child("if"),
					sub.If, misplacedConditionMessage))
			}
		}
	}
	return errs
}

// misplacedCondition reports an if with a $( ) outside its CUE strings that is
// not one $( ) expression. A $$( is an escape, not an expression.
func misplacedCondition(cond string) bool {
	return !wfexpressions.IsCondition(cond) && expressionOutsideStrings(cond)
}

// expressionOutsideStrings reports a $( ) in CUE source outside its string and
// bytes literals.
func expressionOutsideStrings(src string) bool {
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '"', '\'':
			i = endOfLiteral(src, i, c)
		case '$':
			if strings.HasPrefix(src[i:], "$$(") {
				i += 2
				continue
			}
			if strings.HasPrefix(src[i:], "$(") {
				return true
			}
		}
	}
	return false
}

// endOfLiteral returns the index of the quote that closes the literal opening
// at src[start], or the last index when it is unclosed.
func endOfLiteral(src string, start int, quote byte) int {
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			i++
		case quote:
			return i
		}
	}
	return len(src) - 1
}
