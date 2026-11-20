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
	wfTypesv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/nesting"
)

// ValidateCUEDepth refuses an Application whose workflow conditions or data passing carry CUE
// nested deeper than nesting.MaxDepth: the controller compiles them on every reconcile.
func ValidateCUEDepth(app *v1beta1.Application) field.ErrorList {
	var errs field.ErrorList
	components := field.NewPath("spec", "components")
	for i, comp := range app.Spec.Components {
		errs = append(errs, dataPassingDepth(components.Index(i), comp.Inputs, comp.Outputs)...)
	}
	if app.Spec.Workflow != nil {
		errs = append(errs, StepsCUEDepth(field.NewPath("spec", "workflow", "steps"), app.Spec.Workflow.Steps)...)
	}
	return errs
}

// StepsCUEDepth refuses workflow steps, and their sub-steps, whose condition or data passing
// carries CUE nested deeper than nesting.MaxDepth.
func StepsCUEDepth(path *field.Path, steps []wfTypesv1alpha1.WorkflowStep) field.ErrorList {
	var errs field.ErrorList
	for i, step := range steps {
		errs = append(errs, stepCUEDepth(path.Index(i), step.WorkflowStepBase)...)
		for j, sub := range step.SubSteps {
			errs = append(errs, stepCUEDepth(path.Index(i).Child("subSteps").Index(j), sub)...)
		}
	}
	return errs
}

func stepCUEDepth(path *field.Path, step wfTypesv1alpha1.WorkflowStepBase) field.ErrorList {
	errs := cueDepth(path.Child("if"), step.If)
	return append(errs, dataPassingDepth(path, step.Inputs, step.Outputs)...)
}

func dataPassingDepth(path *field.Path, inputs wfTypesv1alpha1.StepInputs, outputs wfTypesv1alpha1.StepOutputs) field.ErrorList {
	var errs field.ErrorList
	for i, in := range inputs {
		errs = append(errs, cueDepth(path.Child("inputs").Index(i).Child("from"), in.From)...)
		errs = append(errs, cueDepth(path.Child("inputs").Index(i).Child("parameterKey"), in.ParameterKey)...)
	}
	for i, out := range outputs {
		errs = append(errs, cueDepth(path.Child("outputs").Index(i).Child("valueFrom"), out.ValueFrom)...)
		errs = append(errs, cueDepth(path.Child("outputs").Index(i).Child("name"), out.Name)...)
	}
	return errs
}

// cueDepth refuses src at path when it nests too deeply. The value itself is not repeated in the
// error: it may be large.
func cueDepth(path *field.Path, src string) field.ErrorList {
	if src == "" {
		return nil
	}
	if err := nesting.Check(src, nesting.MaxDepth); err != nil {
		return field.ErrorList{field.Forbidden(path, err.Error())}
	}
	return nil
}
