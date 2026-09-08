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

	"github.com/oam-dev/kubevela/pkg/module/service/api"
)

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
	assert.Equal(t, app, out.Returns.Application)
}

// TestRender_DefaultsVersionToEmpty asserts an omitted Version reaches the
// render service as "" (latest), not some other zero value.
func TestRender_DefaultsVersionToEmpty(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{res: &api.ModuleResult{Application: map[string]interface{}{}}}
	api.SetDefaultRenderer(fake)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.NoError(t, err)
	assert.Equal(t, "", fake.req.Version)
}

func TestRender_ErrorsWhenRendererNotInitialized(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestRender_PropagatesRendererError(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{err: errors.New("module not found in registry")})

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "nope"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module not found in registry")
}

func TestRender_ErrorsOnNilResult(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })
	api.SetDefaultRenderer(&fakeRenderer{}) // zero value: returns (nil, nil)

	_, err := Render(context.Background(), &RenderParams{Params: RenderVars{Module: "s3"}})
	require.Error(t, err)
}
