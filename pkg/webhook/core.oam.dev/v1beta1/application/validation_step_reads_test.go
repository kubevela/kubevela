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

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func stepReadsApp(steps ...wfv1alpha1.WorkflowStep) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default",
			Annotations: map[string]string{oam.AnnotationCelExpressions: "true"}},
		Spec: v1beta1.ApplicationSpec{Workflow: &v1beta1.Workflow{Steps: steps}},
	}
}

func readStep(name, props string, outputs ...string) wfv1alpha1.WorkflowStep {
	s := wfv1alpha1.WorkflowStep{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: name, Type: "notification"}}
	if props != "" {
		s.Properties = &runtime.RawExtension{Raw: []byte(props)}
	}
	for _, o := range outputs {
		s.Outputs = append(s.Outputs, wfv1alpha1.OutputItem{Name: o, ValueFrom: "value"})
	}
	return s
}

func TestValidateWorkflowStepReads(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, true)
	h := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()}

	loop := wfv1alpha1.WorkflowStep{
		WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "per-tenant", Type: "step-group"},
		ForEach:          &wfv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(`"$(steps.list.outputs.tenants)"`)}},
		SubSteps: []wfv1alpha1.WorkflowStepBase{readStep("greet",
			`{"message":"hi $(each.item.name), image $(steps.build.outputs.image)"}`).WorkflowStepBase},
	}
	t.Run("reads of earlier steps and the iteration are allowed", func(t *testing.T) {
		app := stepReadsApp(readStep("build", "", "image"), readStep("list", "", "tenants"), loop)
		require.Empty(t, h.ValidateWorkflow(context.Background(), app))
		require.Empty(t, h.ValidateSources(context.Background(), app), "and they plan and type like any expression")
	})
	t.Run("a read of a later step in a workflow run in order is refused", func(t *testing.T) {
		errs := h.ValidateWorkflow(context.Background(), stepReadsApp(
			readStep("notify", `{"message":"$(steps.build.outputs.image)"}`), readStep("build", "", "image")))
		require.NotEmpty(t, errs)
		require.Contains(t, errs.ToAggregate().Error(), "build runs after notify")
	})
	t.Run("each outside a forEach body is refused", func(t *testing.T) {
		errs := h.ValidateWorkflow(context.Background(), stepReadsApp(readStep("notify", `{"message":"$(each.item)"}`)))
		require.NotEmpty(t, errs)
		require.Contains(t, errs.ToAggregate().Error(), "each is only set inside a forEach body")
	})
	t.Run("a component read in a step is refused", func(t *testing.T) {
		app := stepReadsApp(readStep("notify", `{"message":"$(component.db.output.status.endpoint)"}`))
		errs := append(h.ValidateWorkflow(context.Background(), app), h.ValidateSources(context.Background(), app)...)
		require.NotEmpty(t, errs)
		require.Contains(t, errs.ToAggregate().Error(), `"component" cannot be read here`)
	})
	t.Run("an Application that has not opted in is not read", func(t *testing.T) {
		app := stepReadsApp(readStep("notify", `{"message":"$(each.item)"}`))
		app.Annotations = nil
		require.Empty(t, h.ValidateWorkflow(context.Background(), app))
	})
}
