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

package appfile

import (
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/sources"
)

func enableExpressions(t *testing.T) {
	t.Helper()
	before := map[string]bool{
		string(features.EnableCelExpressions):      utilfeature.DefaultMutableFeatureGate.Enabled(features.EnableCelExpressions),
		string(features.RequireCelExpressionOptIn): utilfeature.DefaultMutableFeatureGate.Enabled(features.RequireCelExpressionOptIn),
	}
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.EnableCelExpressions): true, string(features.RequireCelExpressionOptIn): true}))
	t.Cleanup(func() { require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(before)) })
}

var optedIn = map[string]string{oam.AnnotationCelExpressions: "true"}

func TestStepsWithReadDependencies(t *testing.T) {
	enableExpressions(t)
	reads := func(name string, exprs ...string) common.ApplicationComponent {
		props := map[string]interface{}{}
		for i, e := range exprs {
			props[string(rune('a'+i))] = `$(component.` + e + `.metadata.name)`
		}
		return comp(t, name, props)
	}
	step := func(name, typ string, props map[string]interface{}, dependsOn ...string) wfTypesv1alpha1.WorkflowStep {
		s := wfTypesv1alpha1.WorkflowStep{WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name: name, Type: typ, DependsOn: dependsOn}}
		if props != nil {
			s.Properties = rawProps(t, props)
		}
		return s
	}
	apply := func(c string, dependsOn ...string) wfTypesv1alpha1.WorkflowStep {
		return step(c, "apply-component", map[string]interface{}{"component": c}, dependsOn...)
	}
	group := func(name string, subs ...wfTypesv1alpha1.WorkflowStep) wfTypesv1alpha1.WorkflowStep {
		g := step(name, "step-group", nil)
		for _, s := range subs {
			g.SubSteps = append(g.SubSteps, s.WorkflowStepBase)
		}
		return g
	}
	dag := &wfTypesv1alpha1.WorkflowExecuteMode{Steps: "DAG", SubSteps: "DAG"}
	stepByStep := &wfTypesv1alpha1.WorkflowExecuteMode{Steps: "StepByStep", SubSteps: "DAG"}
	subStepByStep := &wfTypesv1alpha1.WorkflowExecuteMode{Steps: "StepByStep", SubSteps: "StepByStep"}
	dbAndAPI := []common.ApplicationComponent{reads("db"), reads("api", "db.output")}

	type deps map[string][]string
	for _, tc := range []struct {
		name  string
		comps []common.ApplicationComponent
		mode  *wfTypesv1alpha1.WorkflowExecuteMode
		steps []wfTypesv1alpha1.WorkflowStep
		want  deps
	}{
		{name: "generated workflow: one step per component, named after it",
			comps: dbAndAPI, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db"), apply("api")},
			want:  deps{"api": {"db"}}},
		{name: "a written dependsOn is kept and not repeated",
			comps: []common.ApplicationComponent{reads("db"), reads("cfg"), reads("api", "db.output")}, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db"), apply("cfg"), apply("api", "cfg", "db")},
			want:  deps{"api": {"cfg", "db"}}},
		{name: "no step named after the producer: no edge",
			comps: dbAndAPI, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{step("setup-db", "apply-component", map[string]interface{}{"component": "db"}), apply("api")},
			want:  deps{}},
		{name: "a read that names a placement implies no edge",
			comps: []common.ApplicationComponent{reads("db"), reads("api", `db.cluster("east").output`)}, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db"), apply("api")},
			want:  deps{}},
		{name: "a reader in a deploy step gets no step edge; the step orders its own components",
			comps: dbAndAPI, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db"), step("deploy-api", "deploy", map[string]interface{}{"policies": []string{}})},
			want:  deps{}},
		{name: "in order, no edge to a later step",
			comps: dbAndAPI, mode: stepByStep,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("api"), apply("db")},
			want:  deps{}},
		{name: "an edge that would close a cycle with a written dependsOn is left out",
			comps: dbAndAPI, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db", "api"), apply("api")},
			want:  deps{"db": {"api"}}},
		{name: "sibling sub-steps",
			comps: dbAndAPI, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{group("g", apply("db"), apply("api"))},
			want:  deps{"api": {"db"}}},
		{name: "sibling sub-steps in order: no edge to a later sibling",
			comps: dbAndAPI, mode: subStepByStep,
			steps: []wfTypesv1alpha1.WorkflowStep{group("g", apply("api"), apply("db"))},
			want:  deps{}},
		{name: "a sub-step reading another step's component: the group waits",
			comps: []common.ApplicationComponent{reads("db"), reads("cache"), reads("api", "db.output")}, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{apply("db"), group("g", apply("cache"), apply("api"))},
			want:  deps{"g": {"db"}}},
		{name: "a step reading a sub-step's component waits on the group",
			comps: []common.ApplicationComponent{reads("db"), reads("cache"), reads("api", "db.output")}, mode: dag,
			steps: []wfTypesv1alpha1.WorkflowStep{group("g", apply("db"), apply("cache")), apply("api")},
			want:  deps{"api": {"g"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			af := &Appfile{AppAnnotations: optedIn, Components: tc.comps, WorkflowMode: tc.mode}
			af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)
			in := make([]wfTypesv1alpha1.WorkflowStep, len(tc.steps))
			for i := range tc.steps {
				in[i] = *tc.steps[i].DeepCopy()
			}
			out := af.StepsWithReadDependencies(in)
			require.Equal(t, tc.steps, in, "the steps passed in are not changed")

			got := deps{}
			for _, s := range out {
				if len(s.DependsOn) > 0 {
					got[s.Name] = s.DependsOn
				}
				for _, sub := range s.SubSteps {
					if len(sub.DependsOn) > 0 {
						got[sub.Name] = sub.DependsOn
					}
				}
			}
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("an Application that has not opted in gets no edges", func(t *testing.T) {
		af := &Appfile{Components: dbAndAPI, WorkflowMode: dag}
		af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)
		steps := []wfTypesv1alpha1.WorkflowStep{apply("db"), apply("api")}
		require.Equal(t, steps, af.StepsWithReadDependencies(steps))
	})
}

func TestComponentsWithReadDependencies(t *testing.T) {
	enableExpressions(t)
	reads := func(name, props string, dependsOn ...string) common.ApplicationComponent {
		return common.ApplicationComponent{Name: name, Type: "webservice", DependsOn: dependsOn,
			Properties: &runtime.RawExtension{Raw: []byte(props)}}
	}
	written := append(make([]string, 0, 4), "cfg")
	components := []common.ApplicationComponent{
		reads("cache", `{}`),
		reads("flags", `{}`),
		reads("cfg", `{}`),
		reads("api", `{"cache":"$(component.cache.output.metadata.name)",`+
			`"flags":"$(component.flags.cluster(\"east\").output.metadata.name)",`+
			`"db":"$(component.db.output.metadata.name)"}`, written...),
	}
	af := &Appfile{AppAnnotations: optedIn, Components: components}
	af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)

	got := af.ComponentsWithReadDependencies(components)
	require.Equal(t, []string{"cfg", "cache"}, got[3].DependsOn,
		"a read beside the reader, of a component in this step; not one naming a placement or outside the step")
	require.Equal(t, []string{"cfg"}, components[3].DependsOn, "the components passed in are not changed")
	require.Empty(t, written[1:2][0], "nor the array behind their dependsOn")
	require.Empty(t, got[0].DependsOn)

	af.AppAnnotations = nil
	af.Dependencies = sources.Dependencies(af.Components, af.AppAnnotations)
	require.Equal(t, []string{"cfg"}, af.ComponentsWithReadDependencies(components)[3].DependsOn,
		"an Application that has not opted in reads nothing")
}
