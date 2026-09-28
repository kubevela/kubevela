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

	steps := []wfTypesv1alpha1.WorkflowStep{{
		WorkflowStepBase: wfTypesv1alpha1.WorkflowStepBase{
			Name:       "scale",
			Type:       "scale-component",
			Properties: &runtime.RawExtension{Raw: []byte(`{"component":"$(context.appName)"}`)},
		},
		ForEach: &wfTypesv1alpha1.ForEach{Items: &apiextensionsv1.JSON{Raw: []byte(`["$(context.namespace)"]`)}},
	}}
	calls := map[string]int{}
	record := func(name, _ string, _ map[string]sources.SourceResolutionStatus) { calls[name]++ }
	require.NoError(t, resolveWorkflowStepSources(&appfile.Appfile{Name: "web", Namespace: "prod"}, steps, record))
	// A reader is recorded under its step name, and a second record replaces the first,
	// so what the properties read and what forEach.items read must arrive together.
	require.Equal(t, map[string]int{"scale": 1}, calls)
	require.JSONEq(t, `{"component":"web"}`, string(steps[0].Properties.Raw))
	require.JSONEq(t, `["prod"]`, string(steps[0].ForEach.Items.Raw))
}
