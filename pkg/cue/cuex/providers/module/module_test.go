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

package module

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/cue/cuex/providers/validation"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
)

// enableModuleComponent turns the gate on for tests that exercise the happy path.
// Feature gates default to false, and Render refuses outright when the gate is off.
func enableModuleComponent(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableModuleComponent, true)
}

type fakeRenderer struct {
	req api.ModuleRequest
	res *api.ModuleResult
	err error
}

func (f *fakeRenderer) RenderModule(_ context.Context, req api.ModuleRequest) (*api.ModuleResult, error) {
	f.req = req
	return f.res, f.err
}

func TestRender_PassesParamsThroughAndReturnsApplication(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	app := map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "module-s3"},
	}
	fake := &fakeRenderer{res: &api.ModuleResult{Application: app}}
	api.SetDefaultRenderer(fake)

	out, err := Render(context.Background(), &RenderParams{
		Params: RenderVars{Module: "s3", Registry: "catalog", Namespace: "team-a", Version: "1.2.0"},
	})
	require.NoError(t, err)

	assert.Equal(t, "s3", fake.req.Module)
	assert.Equal(t, "catalog", fake.req.Registry)
	assert.Equal(t, "team-a", fake.req.Namespace)
	assert.Equal(t, "1.2.0", fake.req.Version)
	assert.Nil(t, fake.req.Placement)
	assert.Equal(t, app, out.Returns.Application)
}

func TestRender_PassesPlacement(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	place := &v1alpha1.Placement{Clusters: []string{"spoke"}}
	_, err := Render(context.Background(), &RenderParams{
		Params: RenderVars{Module: "store", Registry: "modules", Version: "1.0.1", Placement: place},
	})
	require.NoError(t, err)
	assert.Equal(t, place, fake.req.Placement)
	assert.Equal(t, []string{"spoke"}, fake.req.Placement.Clusters)
}

// TestRender_DefaultsVersionToEmpty asserts an omitted Version reaches the
// render service as "" (latest), not some other zero value.
func TestRender_DefaultsVersionToEmpty(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.NoError(t, err)
	assert.Equal(t, "", fake.req.Version)
}

// TestRender_ErrorsWhenModuleComponentGateDisabled asserts Render reports the
// EnableModuleComponent gate by name, rather than falling through to the
// unhelpful "module renderer not initialized" error, when the gate is off.
func TestRender_ErrorsWhenModuleComponentGateDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableModuleComponent, false)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	// A working renderer is installed, so any failure here can only come from the gate.
	fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableModuleComponent")
	assert.Equal(t, api.ModuleRequest{}, fake.req, "the renderer must not be reached when the gate is off")
}

func TestRender_ErrorsWhenRendererNotInitialized(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestRender_PropagatesRendererError(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{err: errors.New("module not found in registry")})

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "nope"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module not found in registry")
}

func TestRender_ErrorsOnNilResult(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{}) // zero value: returns (nil, nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.Error(t, err)
}

func TestRender_IsRefusedWhenTheGateIsDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableModuleComponent, false)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	// A working renderer is installed, so any failure here can only come from the gate.
	fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableModuleComponent")
	assert.Equal(t, api.ModuleRequest{}, fake.req, "the renderer must not be reached when the gate is off")
}

// TestRender_RejectsInvalidPlacement asserts the provider refuses a placement
// that names neither or both of clusters and clusterLabelSelector, with a
// plain message, before the renderer is reached. It runs under a
// validation-only context too, so admission rejects the same request.
func TestRender_RejectsInvalidPlacement(t *testing.T) {
	enableModuleComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	testCases := map[string]struct {
		place *v1alpha1.Placement
		want  string
	}{
		"neither": {&v1alpha1.Placement{}, "placement must set clusters or clusterLabelSelector"},
		"both": {&v1alpha1.Placement{Clusters: []string{"spoke"}, ClusterLabelSelector: map[string]string{"tier": "runtime"}},
			"placement must set clusters or clusterLabelSelector, not both"},
	}
	contexts := map[string]context.Context{
		"render":    context.Background(),
		"admission": validation.WithValidationOnly(context.Background()),
	}
	for name, tc := range testCases {
		for ctxName, ctx := range contexts {
			t.Run(name+"/"+ctxName, func(t *testing.T) {
				fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
				api.SetDefaultRenderer(fake)

				_, err := Render(ctx, &RenderParams{
					Params: RenderVars{Module: "store", Registry: "modules", Placement: tc.place},
				})

				require.Error(t, err)
				assert.Equal(t, tc.want, err.Error())
				assert.Equal(t, api.ModuleRequest{}, fake.req, "the renderer must not be reached")
			})
		}
	}
}
