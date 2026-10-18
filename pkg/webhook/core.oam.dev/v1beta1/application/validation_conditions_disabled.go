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
