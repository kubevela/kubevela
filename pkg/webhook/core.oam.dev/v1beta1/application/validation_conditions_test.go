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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestStepConditionsAreCheckedLikeProperties(t *testing.T) {
	defs := append(readKindsDefinitions(), &v1beta1.SourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "flags", Namespace: "vela-system"},
		Spec: v1beta1.SourceDefinitionSpec{Schematic: &common.Schematic{CUE: &common.CUE{Template: `
schema: {enabled: bool, region: string}
output: {enabled: true, region: "eu"}
parameter: {}
`}}},
	})
	step := func(name, cond string) wfv1alpha1.WorkflowStep {
		return wfv1alpha1.WorkflowStep{WorkflowStepBase: wfv1alpha1.WorkflowStepBase{
			Name: name, Type: "notify", If: cond, Properties: rawJSON(`{"text":"hi"}`)}}
	}
	group := func(items, cond string) wfv1alpha1.WorkflowStep {
		return wfv1alpha1.WorkflowStep{
			WorkflowStepBase: wfv1alpha1.WorkflowStepBase{Name: "per-region", Type: "step-group"},
			ForEach:          &wfv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(items)}},
			SubSteps: []wfv1alpha1.WorkflowStepBase{{Name: "deploy", Type: "notify", If: cond,
				Properties: rawJSON(`{"text":"hi"}`)}},
		}
	}
	for _, tc := range []struct {
		name    string
		steps   []wfv1alpha1.WorkflowStep
		refused string
	}{
		{name: "a bool source field",
			steps: []wfv1alpha1.WorkflowStep{step("notify", "$(source.f.enabled)")}},
		{name: "a comparison",
			steps: []wfv1alpha1.WorkflowStep{step("notify", `$(context.appName == "app" && source.f.region != "us")`)}},
		{name: "a step's status",
			steps: []wfv1alpha1.WorkflowStep{step("build", ""), step("notify", "$(steps.build.succeeded)")}},
		{name: "a CUE if is left alone",
			steps: []wfv1alpha1.WorkflowStep{step("notify", `status.build.succeeded && inputs.x == "$(y)"`)}},
		{name: "a string source field is refused",
			steps:   []wfv1alpha1.WorkflowStep{step("notify", "$(source.f.region)")},
			refused: "spec.workflow.steps[0].if: Invalid value: \"$(source.f.region)\": type mismatch: expression $(source.f.region) is string but if expects bool"},
		{name: "context that is not a bool is refused",
			steps:   []wfv1alpha1.WorkflowStep{step("notify", "$(context.appName)")},
			refused: "is string but if expects bool"},
		{name: "a source not declared is refused",
			steps:   []wfv1alpha1.WorkflowStep{step("notify", "$(source.nope.enabled)")},
			refused: "source is not declared in spec.sources"},
		{name: "a component read is refused on a step",
			steps:   []wfv1alpha1.WorkflowStep{step("notify", `$(component.db.output.metadata.name == "db")`)},
			refused: "spec.workflow.steps[0].if"},
		{name: "a bool loop item, in a looping group's sub-step",
			steps: []wfv1alpha1.WorkflowStep{group(`[true, false]`, "$(each.item)")}},
		{name: "a string loop item is refused",
			steps:   []wfv1alpha1.WorkflowStep{group(`["eu", "us"]`, "$(each.item)")},
			refused: "spec.workflow.steps[0].subSteps[0].if: Invalid value: \"$(each.item)\": type mismatch: expression $(each.item) is string but if expects bool"},
		{name: "the index is an int, not a bool",
			steps:   []wfv1alpha1.WorkflowStep{group(`["eu", "us"]`, "$(each.index)")},
			refused: "is int but if expects bool"},
		{name: "a comparison on the index",
			steps: []wfv1alpha1.WorkflowStep{group(`["eu", "us"]`, "$(each.index > 0)")}},
		{name: "a field some loop items leave out is refused unguarded",
			steps:   []wfv1alpha1.WorkflowStep{group(`[{"wave":1},{"region":"us"}]`, "$(each.item.wave > 0)")},
			refused: "each.item.wave may be absent and decides the step"},
		{name: "and admitted guarded",
			steps: []wfv1alpha1.WorkflowStep{group(`[{"wave":1},{"region":"us"}]`, "$(has(each.item.wave) && each.item.wave > 0)")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewClientBuilder().WithScheme(readKindsScheme()).WithRuntimeObjects(defs...).Build()
			app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
				Spec: v1beta1.ApplicationSpec{
					Components: []common.ApplicationComponent{{Name: "db", Type: "server", Properties: rawJSON(`{}`)}},
					Sources:    []v1beta1.ApplicationSource{{Name: "f", Type: "flags", Properties: rawJSON(`{}`)}},
					Workflow:   &v1beta1.Workflow{Steps: tc.steps},
				}}
			errs := (&ValidatingHandler{Client: cli}).ValidateSources(context.Background(), app)
			if tc.refused == "" {
				require.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1, "errors: %v", errs)
			require.Contains(t, errs[0].Error(), tc.refused)
		})
	}
}

var _ runtime.Object = &v1beta1.Application{}
