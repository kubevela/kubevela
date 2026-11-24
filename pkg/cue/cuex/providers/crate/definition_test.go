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

package crate_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	"github.com/oam-dev/kubevela/pkg/crate/service/api"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/definition/deftest"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/oam"
)

const crateDefinitionPath = "../../../../../vela-templates/definitions/internal/component/crate.cue"

type capturingRenderer struct{ req api.CrateRequest }

func (c *capturingRenderer) RenderCrate(_ context.Context, req api.CrateRequest) (*api.CrateResult, error) {
	c.req = req
	return &api.CrateResult{Application: map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "crate-" + req.Crate},
	}}, nil
}

// renderCrate evaluates the crate ComponentDefinition's template for one
// component and returns the request it sent to the crate renderer.
func renderCrate(t *testing.T, data process.ContextData, params map[string]interface{}) (api.CrateRequest, error) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCrateComponent, true)
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	fake := &capturingRenderer{}
	api.SetDefaultRenderer(fake)

	data.CompName = "widget-kit"
	data.AppName = "installer"
	data.Ctx = context.Background()
	err := definition.NewWorkloadAbstractEngine(data.CompName).Complete(process.NewContext(data), deftest.Template(t, crateDefinitionPath), params)
	return fake.req, err
}

func TestCrateDefinition_DerivesNamespaceFromInstallingApplication(t *testing.T) {
	cases := map[string]struct {
		data          process.ContextData
		wantNamespace string
	}{
		"user Application in its own namespace": {
			data:          process.ContextData{Namespace: "foo"},
			wantNamespace: "foo",
		},
		"user Application in vela-system": {
			data:          process.ContextData{Namespace: "vela-system"},
			wantNamespace: "vela-system",
		},
		"owned addon Application carries the installing namespace": {
			data: process.ContextData{
				Namespace:      "vela-system",
				AppLabels:      map[string]string{oam.LabelAddonName: "tenant-widgets"},
				AppAnnotations: map[string]string{oam.AnnotationCrateInstallNamespace: "kit-tenant"},
			},
			wantNamespace: "kit-tenant",
		},
		// Only the addon renderer may redirect a crate: on a user's own
		// Application the annotation is ignored, or it would reopen the
		// arbitrary-namespace hole the parameter's removal closes.
		"annotation on a user Application is ignored": {
			data: process.ContextData{
				Namespace:      "foo",
				AppAnnotations: map[string]string{oam.AnnotationCrateInstallNamespace: "elsewhere"},
			},
			wantNamespace: "foo",
		},
		"owned addon Application without the annotation uses its own namespace": {
			data: process.ContextData{
				Namespace: "vela-system",
				AppLabels: map[string]string{oam.LabelAddonName: "tenant-widgets"},
			},
			wantNamespace: "vela-system",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req, err := renderCrate(t, tc.data, map[string]interface{}{"version": "1.0.0"})
			require.NoError(t, err)
			assert.Equal(t, tc.wantNamespace, req.Namespace)
			assert.Equal(t, "widget-kit", req.Crate)
		})
	}
}

func TestCrateDefinition_RejectsNamespaceProperty(t *testing.T) {
	_, err := renderCrate(t, process.ContextData{Namespace: "foo"},
		map[string]interface{}{"namespace": "elsewhere"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "namespace is no longer accepted")
}
