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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	wfv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func forEachApp(items string) *v1beta1.Application {
	return &v1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec: v1beta1.ApplicationSpec{
			Sources: []v1beta1.ApplicationSource{{Name: "inv", Type: "inventory"}},
			Workflow: &v1beta1.Workflow{
				Steps: []wfv1alpha1.WorkflowStep{{
					WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "scale", Type: "scale-component"},
					ForEach:          &wfv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(items)}},
				}},
			},
		},
	}
}

func TestValidateSourcesForEachItems(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	inventory := &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "inventory", Namespace: "default"},
		Spec: v1beta1.SourceDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
schema: {
  clusters: [...{name: string}]
  region:   string
}
output: {clusters: [], region: "eu"}
parameter: {}
`}}},
	}
	handler := &ValidatingHandler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(inventory).Build()}

	cases := map[string]struct {
		items string
		want  string
	}{
		"a list-typed read":                   {items: `"$(source.inv.clusters)"`},
		"expressions inside a literal list":   {items: `["$(source.inv.region)", "west"]`},
		"a literal list":                      {items: `["east", "west"]`},
		"a string-typed read":                 {items: `"$(source.inv.region)"`, want: "spec.workflow.steps[0].forEach.items"},
		"a path the schema does not have":     {items: `"$(source.inv.nosuch)"`, want: "spec.workflow.steps[0].forEach.items"},
		"a source that is not declared":       {items: `"$(source.other.clusters)"`, want: "not declared in spec.sources"},
		"an expression that does not compile": {items: `"$(source.inv.)"`, want: "spec.workflow.steps[0].forEach.items"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			errs := handler.ValidateSources(context.Background(), forEachApp(tc.items))
			if tc.want == "" {
				require.Empty(t, errs)
				return
			}
			require.NotEmpty(t, errs)
			require.Contains(t, errs.ToAggregate().Error(), tc.want)
		})
	}

	t.Run("a string-typed read names the list it needed", func(t *testing.T) {
		errs := handler.ValidateSources(context.Background(), forEachApp(`"$(source.inv.region)"`))
		require.Contains(t, errs.ToAggregate().Error(), "forEach.items expects list")
	})
}

func TestValidateWorkflowForEach(t *testing.T) {
	h := &ValidatingHandler{}
	t.Run("an expression is allowed where expressions are enabled", func(t *testing.T) {
		require.Empty(t, h.ValidateWorkflow(context.Background(), forEachApp(`"$(source.inv.clusters)"`)))
	})
	t.Run("the shared rules apply under spec.workflow.steps", func(t *testing.T) {
		app := forEachApp(`["a"]`)
		app.Spec.Workflow.Steps[0].ForEach.From = "regions"
		errs := h.ValidateWorkflow(context.Background(), app)
		require.NotEmpty(t, errs)
		require.Contains(t, errs.ToAggregate().Error(), "spec.workflow.steps[0].forEach")
		require.Contains(t, errs.ToAggregate().Error(), "exactly one of items and from")
	})
}
