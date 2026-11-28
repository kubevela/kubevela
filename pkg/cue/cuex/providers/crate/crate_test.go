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

package crate

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	v1alpha1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/crate/service/api"
	"github.com/oam-dev/kubevela/pkg/features"
)

// enableCrateComponent turns the gate on for tests that exercise the happy path.
// Feature gates default to false, and Render refuses outright when the gate is off.
func enableCrateComponent(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCrateComponent, true)
}

type fakeRenderer struct {
	req api.CrateRequest
	res *api.CrateResult
	err error
}

func (f *fakeRenderer) RenderCrate(_ context.Context, req api.CrateRequest) (*api.CrateResult, error) {
	f.req = req
	return f.res, f.err
}

func TestRender_PassesParamsThroughAndReturnsApplication(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	app := map[string]interface{}{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]interface{}{"name": "crate-s3"},
	}
	fake := &fakeRenderer{res: &api.CrateResult{Application: app}}
	api.SetDefaultRenderer(fake)

	out, err := Render(context.Background(), &RenderParams{
		Params: RenderVars{Crate: "s3", Registry: "catalog", Namespace: "team-a", Version: "1.2.0"},
	})
	require.NoError(t, err)

	assert.Equal(t, "s3", fake.req.Crate)
	assert.Equal(t, "catalog", fake.req.Registry)
	assert.Equal(t, "team-a", fake.req.Namespace)
	assert.Equal(t, "1.2.0", fake.req.Version)
	assert.Nil(t, fake.req.Placement)
	assert.Equal(t, app, out.Returns.Application)
}

func TestRender_PassesPlacement(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{res: &api.CrateResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	place := &v1alpha1.Placement{Clusters: []string{"spoke"}}
	_, err := Render(context.Background(), &RenderParams{
		Params: RenderVars{Crate: "store", Registry: "crates", Version: "1.0.1", Placement: place},
	})
	require.NoError(t, err)
	assert.Equal(t, place, fake.req.Placement)
	assert.Equal(t, []string{"spoke"}, fake.req.Placement.Clusters)
}

// TestRender_DefaultsVersionToEmpty asserts an omitted Version reaches the
// render service as "" (latest), not some other zero value.
func TestRender_DefaultsVersionToEmpty(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{res: &api.CrateResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "s3"}})
	require.NoError(t, err)
	assert.Equal(t, "", fake.req.Version)
}

// TestRender_ErrorsWhenCrateComponentGateDisabled asserts Render reports the
// EnableCrateComponent gate by name, rather than falling through to the
// unhelpful "crate renderer not initialized" error, when the gate is off.
func TestRender_ErrorsWhenCrateComponentGateDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCrateComponent, false)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	// A working renderer is installed, so any failure here can only come from the gate.
	fake := &fakeRenderer{res: &api.CrateResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "s3"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableCrateComponent")
	assert.Equal(t, api.CrateRequest{}, fake.req, "the renderer must not be reached when the gate is off")
}

func TestRender_ErrorsWhenRendererNotInitialized(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "s3"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestRender_PropagatesRendererError(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{err: errors.New("crate not found in registry")})

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "nope"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "crate not found in registry")
}

func TestRender_ErrorsOnNilResult(t *testing.T) {
	enableCrateComponent(t)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{}) // zero value: returns (nil, nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "s3"}})
	require.Error(t, err)
}

func TestRender_IsRefusedWhenTheGateIsDisabled(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCrateComponent, false)

	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	// A working renderer is installed, so any failure here can only come from the gate.
	fake := &fakeRenderer{res: &api.CrateResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Crate: "s3"}})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableCrateComponent")
	assert.Equal(t, api.CrateRequest{}, fake.req, "the renderer must not be reached when the gate is off")
}
