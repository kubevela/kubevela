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

package addon

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/addon/service/api"
)

type fakeRenderer struct {
	req api.AddonRequest
	res *api.AddonResult
	err error
}

func (f *fakeRenderer) RenderAddon(_ context.Context, req api.AddonRequest) (*api.AddonResult, error) {
	f.req = req
	return f.res, f.err
}

func TestRenderPassesThrough(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	fake := &fakeRenderer{
		res: &api.AddonResult{
			ResolvedVersion: "1.2.3",
			Registry:        "my-registry",
			Application: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1beta1",
				"kind":       "Application",
			},
		},
	}
	api.SetDefaultRenderer(fake)

	params := &RenderParams{}
	params.Params = RenderVars{
		Addon:               "example",
		Version:             "1.2.3",
		Registry:            "my-registry",
		Properties:          map[string]interface{}{"foo": "bar"},
		SkipVersionValidate: true,
	}

	got, err := Render(context.Background(), params)
	require.NoError(t, err)

	// request threaded through unchanged
	assert.Equal(t, "example", fake.req.Name)
	assert.Equal(t, "1.2.3", fake.req.Version)
	assert.Equal(t, "my-registry", fake.req.Registry)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, fake.req.Properties)
	assert.True(t, fake.req.SkipVersionValidate)

	// result passed back unchanged
	assert.Equal(t, "1.2.3", got.Returns.ResolvedVersion)
	assert.Equal(t, "my-registry", got.Returns.Registry)
	assert.Equal(t, "Application", got.Returns.Application["kind"])
}

func TestRenderPropagatesError(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	api.SetDefaultRenderer(&fakeRenderer{err: errors.New("boom")})

	params := &RenderParams{}
	params.Params = RenderVars{Addon: "example"}
	_, err := Render(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestRenderErrorsWhenRendererNil(t *testing.T) {
	prev := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(prev) })

	api.SetDefaultRenderer(nil)

	params := &RenderParams{}
	params.Params = RenderVars{Addon: "example"}
	_, err := Render(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}
