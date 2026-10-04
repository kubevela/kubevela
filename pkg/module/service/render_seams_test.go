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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kubevela/pkg/util/singleton"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/module"
	"github.com/oam-dev/kubevela/pkg/module/service/api"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func TestRegisterInstallsTheDefaultRenderer(t *testing.T) {
	previous := api.DefaultRenderer()
	t.Cleanup(func() { api.SetDefaultRenderer(previous) })

	Register()
	r, ok := api.DefaultRenderer().(*rendererImpl)
	require.True(t, ok, "the vela/module provider must find this package's renderer")
	assert.Nil(t, r.fetchFn, "production builds the fetcher per call; nothing is captured at registration")
}

func TestRenderModuleThroughTheFetchSeam(t *testing.T) {
	var asked []string
	r := &rendererImpl{fetchFn: func(_ context.Context, registry, moduleName, version string) (*module.Module, error) {
		asked = []string{registry, moduleName, version}
		return fixtureModule(), nil
	}}

	res, err := r.RenderModule(context.Background(), api.ModuleRequest{Registry: "ecr", Module: "s3", Version: "1.0.0", Namespace: "tenant-a"})
	require.NoError(t, err)
	assert.Equal(t, []string{"ecr", "s3", "1.0.0"}, asked, "the request is passed to the fetcher as given")
	meta := res.Application["metadata"].(map[string]interface{})
	assert.Equal(t, "module-s3", meta["name"])
	assert.Equal(t, types.DefaultKubeVelaNS, meta["namespace"], "the owned Application stays in the system namespace")

	// The definitions install where the request said.
	comps := components(t, res.Application)
	defs := comps[len(comps)-1]["properties"].(map[string]interface{})["objects"].([]interface{})
	defMeta := defs[0].(map[string]interface{})["metadata"].(map[string]interface{})
	assert.Equal(t, "tenant-a", defMeta["namespace"])
}

func TestRenderModuleReportsFetchAndRenderFailures(t *testing.T) {
	boom := errors.New("registry unreachable")
	failing := &rendererImpl{fetchFn: func(context.Context, string, string, string) (*module.Module, error) { return nil, boom }}
	_, err := failing.RenderModule(context.Background(), api.ModuleRequest{Module: "s3"})
	assert.ErrorIs(t, err, boom)

	unrenderable := &rendererImpl{fetchFn: func(context.Context, string, string, string) (*module.Module, error) {
		return &module.Module{Name: "Not_A_DNS_Label"}, nil
	}}
	_, err = unrenderable.RenderModule(context.Background(), api.ModuleRequest{Module: "s3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid module name "Not_A_DNS_Label"`)
}

// TestRenderModuleWithoutSeamReadsTheClusterRegistry runs the production fetch
// path against a fake API server with no module registry ConfigMap: the
// Kubernetes client comes from the process-wide singleton at call time.
func TestRenderModuleWithoutSeamReadsTheClusterRegistry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	singleton.KubeClient.Set(fake.NewClientBuilder().WithScheme(scheme).Build())

	_, err := NewRenderer().RenderModule(context.Background(), api.ModuleRequest{Registry: "ecr-modules", Module: "s3"})
	require.Error(t, err, "no registry is configured in this cluster")
	assert.Contains(t, err.Error(), "ecr-modules")
}

func TestRenderApplicationRejectsAnUnnamedOrMisnamedModule(t *testing.T) {
	_, err := RenderApplication(nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module has no name")

	_, err = RenderApplication(&module.Module{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module has no name")

	_, err = RenderApplication(&module.Module{Name: "S3.Module"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid module name "S3.Module"`)
}

func TestRenderApplicationSkipsALineWithOnlyAuxiliaryObjects(t *testing.T) {
	mod := &module.Module{Name: "s3", Lines: map[string]module.Line{
		"v1": {APIVersion: "v1", Enabled: true, Auxiliary: []map[string]interface{}{auxObject("Composition", "xbuckets-v1")}},
	}}
	app, err := RenderApplication(mod, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"s3-v1-aux"}, names(components(t, app)), "no definitions means no definitions tier")
}

func TestToObjectsAddsMetadataWhenMissing(t *testing.T) {
	out := toObjects([]map[string]interface{}{
		{"apiVersion": "v1", "kind": "ConfigMap"},
		{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "own", "namespace": "elsewhere"}},
	}, "tenant-a")
	require.Len(t, out, 2)
	first := out[0].(map[string]interface{})["metadata"].(map[string]interface{})
	assert.Equal(t, "tenant-a", first["namespace"], "an object without metadata gets the definition namespace")
	second := out[1].(map[string]interface{})["metadata"].(map[string]interface{})
	assert.Equal(t, "elsewhere", second["namespace"], "an object naming its own namespace keeps it")
}

func TestStampIdentityBuildsTheMetadataItNeeds(t *testing.T) {
	// A definition as bare as the parser could ever hand over: no metadata,
	// no labels, no annotations, no spec.
	out := stampIdentity(map[string]interface{}{"kind": "TraitDefinition"}, "s3", "v1", "tenant-a")

	meta := out["metadata"].(map[string]interface{})
	assert.Equal(t, "s3-v1-", meta["name"], "an empty short name still derives deterministically")
	assert.Equal(t, "tenant-a", meta["namespace"])
	labels := meta["labels"].(map[string]interface{})
	assert.Equal(t, "s3", labels[types.LabelDefinitionModule])
	assert.Equal(t, "v1", labels[types.LabelDefinitionModuleAPIVersion])
	assert.Equal(t, "s3", labels[oam.LabelAddonName])
	annos := meta["annotations"].(map[string]interface{})
	assert.Equal(t, "s3-v1-", annos[types.AnnoDefinitionModuleFullName])
	spec := out["spec"].(map[string]interface{})
	assert.Equal(t, "s3", spec["module"])
	assert.Equal(t, "v1", spec["apiVersion"])
}

func TestDeepCopyCoversNestedSlices(t *testing.T) {
	in := map[string]interface{}{
		"list":   []interface{}{map[string]interface{}{"k": "v"}, "plain"},
		"maps":   []map[string]interface{}{{"a": "b"}},
		"scalar": 1,
	}
	out := deepCopyMap(in)

	// Mutate the original through every nesting; the copy must not notice.
	in["list"].([]interface{})[0].(map[string]interface{})["k"] = "changed"
	in["list"].([]interface{})[1] = "changed"
	in["maps"].([]map[string]interface{})[0]["a"] = "changed"

	list := out["list"].([]interface{})
	assert.Equal(t, "v", list[0].(map[string]interface{})["k"])
	assert.Equal(t, "plain", list[1])
	maps := out["maps"].([]interface{})
	assert.Equal(t, "b", maps[0].(map[string]interface{})["a"], "a slice of maps is widened to []interface{} as it is copied")
	assert.Equal(t, 1, out["scalar"])
}
