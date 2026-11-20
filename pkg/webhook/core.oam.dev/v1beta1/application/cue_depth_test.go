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
	"context"
	"encoding/json"
	"strings"
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/cue/nesting"
)

// deepCUE is just over the depth limit, small enough to be cheap to check.
var deepCUE = strings.Repeat("!", nesting.MaxDepth+1) + "true"

func depthApp(mutate func(*v1beta1.Application)) *v1beta1.Application {
	app := &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "team-a"},
		Spec: v1beta1.ApplicationSpec{
			Components: []common.ApplicationComponent{{Name: "c", Type: "webservice",
				Inputs:  wfTypesv1alpha1.StepInputs{{From: "host", ParameterKey: "env.host"}},
				Outputs: wfTypesv1alpha1.StepOutputs{{Name: "host", ValueFrom: "output.status.host"}}}},
			Workflow: &v1beta1.Workflow{Steps: []wfTypesv1alpha1.WorkflowStep{{
				WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: "deploy", If: `status.s.phase != "failed"`,
					Inputs:  wfTypesv1alpha1.StepInputs{{From: "host", ParameterKey: "url"}},
					Outputs: wfTypesv1alpha1.StepOutputs{{Name: "code", ValueFrom: `output.response.code`}}},
				SubSteps: []wfTypesv1alpha1.WorkflowStepBase{{Name: "sub", Type: "deploy", If: "always"}},
			}}},
		},
	}
	if mutate != nil {
		mutate(app)
	}
	return app
}

// The controller compiles a workflow step's condition and its data passing on every reconcile,
// so admission refuses any of them nested too deeply to compile, naming the field.
func TestCUEInAnApplicationIsDepthChecked(t *testing.T) {
	require.Empty(t, ValidateCUEDepth(depthApp(nil)), "ordinary conditions and data passing")

	for path, mutate := range map[string]func(*v1beta1.Application){
		"spec.workflow.steps[0].if": func(a *v1beta1.Application) { a.Spec.Workflow.Steps[0].If = deepCUE },
		"spec.workflow.steps[0].subSteps[0].if": func(a *v1beta1.Application) {
			a.Spec.Workflow.Steps[0].SubSteps[0].If = deepCUE
		},
		"spec.workflow.steps[0].outputs[0].valueFrom": func(a *v1beta1.Application) {
			a.Spec.Workflow.Steps[0].Outputs[0].ValueFrom = deepCUE
		},
		"spec.workflow.steps[0].inputs[0].from": func(a *v1beta1.Application) {
			a.Spec.Workflow.Steps[0].Inputs[0].From = deepCUE
		},
		"spec.components[0].outputs[0].valueFrom": func(a *v1beta1.Application) {
			a.Spec.Components[0].Outputs[0].ValueFrom = deepCUE
		},
		"spec.components[0].inputs[0].parameterKey": func(a *v1beta1.Application) {
			a.Spec.Components[0].Inputs[0].ParameterKey = deepCUE
		},
	} {
		errs := ValidateCUEDepth(depthApp(mutate))
		require.Len(t, errs, 1, path)
		require.Equal(t, path, errs[0].Field)
		require.Contains(t, errs[0].Detail, "CUE is nested deeper than 1000 levels")
	}
}

// The check runs first on create and update, before anything else reads the Application.
func TestApplicationAdmissionRefusesDeepCUEFirst(t *testing.T) {
	h := &ValidatingHandler{Client: fake.NewClientBuilder().Build()}
	deep := depthApp(func(a *v1beta1.Application) { a.Spec.Workflow.Steps[0].If = deepCUE })

	errs := h.ValidateCreate(context.Background(), deep, admission.Request{})
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Detail, "CUE is nested deeper than 1000 levels")

	errs = h.ValidateUpdate(context.Background(), deep, deep.DeepCopy(), admission.Request{})
	require.Len(t, errs, 1)
	require.Contains(t, errs[0].Detail, "CUE is nested deeper than 1000 levels")
}

// A Workflow object an Application references runs the same steps, so it is checked when written.
func TestAReferencedWorkflowIsDepthChecked(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, wfTypesv1alpha1.AddToScheme(scheme))
	h := &ReferencedObjectHandler{app: &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()},
		Decoder: admission.NewDecoder(scheme)}
	write := func(cond string) admission.Response {
		wf := &wfTypesv1alpha1.Workflow{ObjectMeta: metav1.ObjectMeta{Name: "wf", Namespace: "team-a"},
			WorkflowSpec: wfTypesv1alpha1.WorkflowSpec{Steps: []wfTypesv1alpha1.WorkflowStep{{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "s", Type: "suspend", If: cond}}}}}
		raw, err := json.Marshal(wf)
		require.NoError(t, err)
		return h.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create, Namespace: "team-a", Name: "wf",
			Kind:   metav1.GroupVersionKind{Group: "core.oam.dev", Version: "v1alpha1", Kind: "Workflow"},
			Object: runtime.RawExtension{Raw: raw},
		}})
	}
	resp := write(deepCUE)
	require.False(t, resp.Allowed)
	require.Contains(t, resp.Result.Message, "steps[0].if")
	require.Contains(t, resp.Result.Message, "CUE is nested deeper than 1000 levels")
	require.True(t, write("always").Allowed)
}
