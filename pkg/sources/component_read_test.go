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

package sources

import (
	"testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func reader(dependsOn []string, props string, traitProps ...string) common.ApplicationComponent {
	c := common.ApplicationComponent{Name: "api", Type: "webservice", DependsOn: dependsOn,
		Properties: &runtime.RawExtension{Raw: []byte(props)}}
	for _, p := range traitProps {
		c.Traits = append(c.Traits, common.ApplicationTrait{Type: "labels", Properties: &runtime.RawExtension{Raw: []byte(p)}})
	}
	return c
}

func TestReadDependencies(t *testing.T) {
	api := reader(nil,
		`{"db":"$(component.db.output.status.endpoint)","again":"$(component.db.output.metadata.name)",`+
			`"flags":"$(component.flags.cluster(\"east\").output.metadata.name)",`+
			`"q":"$(component.q.namespace(\"infra\").output.metadata.name)",`+
			`"image":"$(source.img.image)"}`,
		`{"svc":"$(component.cache.outputs.svc.spec.clusterIP)"}`)

	require.Equal(t, []string{"cache", "db"}, ReadDependencies(api, nil),
		"each component read beside the reader once, sorted; not one read at a named placement")
}

func TestReadDependenciesNeedTheOptIn(t *testing.T) {
	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.RequireCelExpressionOptIn): true}))
	t.Cleanup(func() {
		require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
			string(features.RequireCelExpressionOptIn): false}))
	})
	api := reader(nil, `{"db":"$(component.db.output.status.endpoint)"}`)

	require.Empty(t, ReadDependencies(api, nil), "without the opt-in, $( ) is ordinary text")
	require.Equal(t, []string{"db"}, ReadDependencies(api, map[string]string{oam.AnnotationCelExpressions: "true"}))
}

func TestEffectiveDependsOn(t *testing.T) {
	written := append(make([]string, 0, 4), "cfg", "db")
	api := reader(written, `{"db":"$(component.db.output.status.endpoint)","c":"$(component.cache.output.metadata.name)"}`)

	require.Equal(t, []string{"cfg", "db", "cache"}, EffectiveDependsOn(api, nil),
		"the written dependsOn, then each read not already there")
	require.Equal(t, []string{"cfg", "db"}, api.DependsOn, "the component is not changed")
	require.Empty(t, written[:3][2], "nor the array behind its dependsOn")

	require.Equal(t, []string{"cfg"}, EffectiveDependsOn(reader([]string{"cfg"}, `{}`), nil))
}

func TestDependencies(t *testing.T) {
	props := func(s string) *runtime.RawExtension { return &runtime.RawExtension{Raw: []byte(s)} }
	comps := []common.ApplicationComponent{
		{Name: "db", Outputs: wfTypesv1alpha1.StepOutputs{{Name: "db-host", ValueFrom: "output.status.endpoint"}}},
		{Name: "cfg"},
		{Name: "api", DependsOn: []string{"cfg"},
			Inputs: wfTypesv1alpha1.StepInputs{{From: "db-host", ParameterKey: "host"}},
			Properties: props(`{"a":"$(component.db.output.status.endpoint)","b":"$(component.db.output.metadata.name)",` +
				`"c":"$(component.flags.cluster(\"east\").output.metadata.name)",` +
				`"d":"$(component.q.namespace(\"infra\").output.metadata.name)",` +
				`"e":"$(component.ops.cluster(\"local\").namespace(\"ops\").output.metadata.name)"}`)},
	}

	require.Equal(t, []common.ComponentDependency{
		{Component: "api", DependsOn: "cfg", Source: common.DependencySourceDependsOn},
		{Component: "api", DependsOn: "db", Source: common.DependencySourceExpression},
		{Component: "api", DependsOn: "db", Source: common.DependencySourceInputs},
		{Component: "api", DependsOn: "flags", Source: common.DependencySourceExpression, Cluster: "east"},
		{Component: "api", DependsOn: "ops", Source: common.DependencySourceExpression, Cluster: "local", Namespace: "ops"},
		{Component: "api", DependsOn: "q", Source: common.DependencySourceExpression, Namespace: "infra"},
	}, Dependencies(comps, nil), "each once, sorted; a placement only where the expression names one")

	require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.RequireCelExpressionOptIn): true}))
	t.Cleanup(func() {
		require.NoError(t, utilfeature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
			string(features.RequireCelExpressionOptIn): false}))
	})
	require.Equal(t, []common.ComponentDependency{
		{Component: "api", DependsOn: "cfg", Source: common.DependencySourceDependsOn},
		{Component: "api", DependsOn: "db", Source: common.DependencySourceInputs},
	}, Dependencies(comps, nil), "without the opt-in, $( ) is ordinary text; dependsOn and inputs still count")
}

func TestDependencyPlacement(t *testing.T) {
	for want, d := range map[string]common.ComponentDependency{
		"":                {},
		"east":            {Cluster: "east"},
		"east/infra":      {Cluster: "east", Namespace: "infra"},
		"namespace infra": {Namespace: "infra"},
	} {
		require.Equal(t, want, DependencyPlacement(d))
	}
}
