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
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	wfTypesv1alpha1 "github.com/kubevela/pkg/apis/oam/v1alpha1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/sources"
)

func TestResolveWorkflowStepSourcesForEachItems(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, false)

	cases := map[string]struct {
		items string
		want  string
	}{
		"a whole expression":            {items: `"$(context.appName)"`, want: `"web"`},
		"a whole expression for a list": {items: `"$([context.appName, \"west\"])"`, want: `["web","west"]`},
		"expressions inside a list":     {items: `["$(context.appName)-east", "$(context.namespace)"]`, want: `["web-east","prod"]`},
		"no expression":                 {items: `["a", 1]`, want: `["a", 1]`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			steps := []wfTypesv1alpha1.WorkflowStep{{
				WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{Name: "scale", Type: "scale-component"},
				ForEach:          &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(tc.items)}},
			}}
			af := &appfile.Appfile{Name: "web", Namespace: "prod"}
			require.NoError(t, resolveWorkflowStepSources(af, steps, nil))
			require.JSONEq(t, tc.want, string(steps[0].ForEach.Items.Raw))
		})
	}
}

func TestResolveWorkflowStepSourcesRecordsAStepOnce(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.EnableCelExpressions, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.RequireCelExpressionOptIn, false)

	source := func(key, output string) *v1beta1.SourceDefinition {
		return &v1beta1.SourceDefinition{Spec: v1beta1.SourceDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{
				Template: "parameter: {}\n$internal: {key: \"" + key + "\", keyInputs: []}\noutput: " + output,
			}},
		}}
	}
	af := &appfile.Appfile{
		Name:      "web",
		Namespace: "prod",
		Sources:   []v1beta1.ApplicationSource{{Name: "app", Type: "app-info"}, {Name: "inv", Type: "inventory"}},
		RelatedSourceDefinitions: map[string]*v1beta1.SourceDefinition{
			"app-info":  source("app-info", `{component: "web"}`),
			"inventory": source("inventory", `{regions: ["east", "west"]}`),
		},
	}
	steps := []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name:       "scale",
			Type:       "scale-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"$(source.app.component)"}`)},
		},
		ForEach: &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(`"$(source.inv.regions)"`)}},
	}}
	var records []map[string]sources.SourceResolutionStatus
	record := func(name, _ string, resolved map[string]sources.SourceResolutionStatus) {
		require.Equal(t, "scale", name)
		records = append(records, resolved)
	}
	require.NoError(t, resolveWorkflowStepSources(af, steps, record))
	require.JSONEq(t, `{"component":"web"}`, string(steps[0].Properties.Raw))
	require.JSONEq(t, `["east","west"]`, string(steps[0].ForEach.Items.Raw))

	// A reader is recorded under its step name, and a second record replaces the first,
	// so what the properties read and what forEach.items read must arrive together.
	require.Len(t, records, 1)
	require.Contains(t, records[0], "app", "the properties' read")
	require.Contains(t, records[0], "inv", "forEach.items' read")
	require.Contains(t, records[0]["app"].ConsumedFields, "component")
	require.Contains(t, records[0]["inv"].ConsumedFields, "regions")
}
