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
	"testing"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func conditionApp(annotations map[string]string, ifs ...string) *v1beta1.Application {
	steps := []wfv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "a", Type: "apply-component"},
	}}
	for i, cond := range ifs {
		steps = append(steps, wfv1alpha1.WorkflowStep{
			WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: string(rune('b' + i)), Type: "apply-component", If: cond},
		})
	}
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", Annotations: annotations},
		Spec:       v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Steps: steps}},
	}
}

// A $( ) if is refused when expressions are off for the Application, naming the
// switch, rather than admitted and then skipped as an invalid CUE condition.
func TestStepConditionsRefusedWithoutExpressions(t *testing.T) {
	h := &ValidatingHandler{}

	t.Run("not opted in", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
		errs := h.ValidateWorkflow(context.Background(), conditionApp(nil, "$(steps.a.succeeded)")).ToAggregate()
		require.Error(t, errs)
		require.Contains(t, errs.Error(), "spec.workflow.steps[1].if")
		require.Contains(t, errs.Error(), oam.AnnotationCelExpressions)
	})

	t.Run("gate off", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, false)
		errs := h.ValidateWorkflow(context.Background(), conditionApp(nil, "$(steps.a.succeeded)")).ToAggregate()
		require.Error(t, errs)
		require.Contains(t, errs.Error(), "EnableCelExpressions feature gate")
	})

	t.Run("a sub-step condition", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
		app := conditionApp(nil)
		app.Spec.Workflow.Steps = append(app.Spec.Workflow.Steps, wfv1alpha1.WorkflowStep{
			WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "group", Type: "step-group"},
			SubSteps:         []wfv1alpha1.WorkflowStepBase{{Name: "inner", Type: "apply-component", If: "$(steps.a.succeeded)"}},
		})
		errs := h.ValidateWorkflow(context.Background(), app).ToAggregate()
		require.Error(t, errs)
		require.Contains(t, errs.Error(), "spec.workflow.steps[1].subSteps[0].if")
	})

	t.Run("CUE conditions are unaffected", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
		require.Empty(t, h.ValidateWorkflow(context.Background(),
			conditionApp(nil, "status.a.succeeded", "always", `inputs.msg == "cost $(5)"`)))
	})

	t.Run("opted in, the condition is admitted", func(t *testing.T) {
		featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
		require.Empty(t, h.ValidateWorkflow(context.Background(),
			conditionApp(map[string]string{oam.AnnotationCelExpressions: "true"}, "$(steps.a.succeeded)")))
	})
}

// An if holding a $( ) that is not the whole of it is refused whether or not the
// Application reads expressions; a $( ) inside a CUE string is text.
func TestMisplacedStepConditionsRefused(t *testing.T) {
	h := &ValidatingHandler{}
	for _, annotations := range []map[string]string{nil, {oam.AnnotationCelExpressions: "true"}} {
		errs := h.ValidateWorkflow(context.Background(),
			conditionApp(annotations, "$(steps.a.succeeded) && $(steps.a.failed)")).ToAggregate()
		require.Error(t, errs)
		require.Contains(t, errs.Error(), "spec.workflow.steps[1].if")
		require.Contains(t, errs.Error(), "must be the whole if")
	}
	require.Empty(t, h.ValidateWorkflow(context.Background(),
		conditionApp(nil, `inputs.msg == "cost $(5)"`, "status.a.succeeded", "status.a.succeeded // see $(docs)")))
	require.Error(t, h.ValidateWorkflow(context.Background(),
		conditionApp(nil, "status.a.succeeded // don't\n&& $(steps.a.succeeded)")).ToAggregate())
	require.Empty(t, h.ValidateWorkflow(context.Background(),
		conditionApp(nil, `x == #"say "$(x)" now"#`, "x == \"\"\"\n\tsay \"$(x)\"\n\t\"\"\"", "x == '''\n\tit's $(x)\n\t'''")))
	require.Error(t, h.ValidateWorkflow(context.Background(),
		conditionApp(nil, `#"a"# && $(steps.a.succeeded)`)).ToAggregate())
}
