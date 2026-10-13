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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/addon/service/api"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module"
	modulesvc "github.com/oam-dev/kubevela/pkg/module/service"
)

// enableModuleComponentForInlineTest turns EnableModuleComponent on; inline
// module rendering refuses outright when the gate is off, the same way the
// external path's own CueX gate check does.
func enableModuleComponentForInlineTest(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableModuleComponent, true)
}

const inlineBucketDefinition = `apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: bucket
spec:
  workload:
    definition:
      apiVersion: v1
      kind: ConfigMap
  schematic:
    cue:
      template: |-
        output: {
        	apiVersion: "v1"
        	kind:       "ConfigMap"
        	metadata: name: context.name
        	data: {}
        }
        parameter: {}
`

func parsedInlineModule(t *testing.T, name string) *module.Module {
	t.Helper()
	mod, err := module.ParseModule(modulesvc.MapFS{
		"_module.cue":                []byte(`module: "` + name + `"` + "\nversion: \"0.1.0\""),
		"v1/_version.cue":            []byte(`apiVersion: "v1"`),
		"v1/definitions/bucket.yaml": []byte(inlineBucketDefinition),
	})
	require.NoError(t, err)
	return mod
}

func renderWith(t *testing.T, pkg pkgaddon.InstallPackage) (map[string]interface{}, error) {
	t.Helper()
	pkg.Meta = pkgaddon.Meta{Name: "inline-addon", Version: "1.0.0"}
	r := &rendererImpl{
		cli: fakeClientWithRegistry(t),
		findPackagesFn: func(_ context.Context, _ client.Client, _, _ []string) ([]*pkgaddon.WholeAddonPackage, error) {
			return []*pkgaddon.WholeAddonPackage{{InstallPackage: pkg, RegistryName: "fixture"}}, nil
		},
	}
	res, err := r.resolveAndRender(context.Background(), api.AddonRequest{Name: "inline-addon", SkipVersionValidate: true})
	if err != nil {
		return nil, err
	}
	return res.Application, nil
}

func componentsByName(t *testing.T, app map[string]interface{}) map[string]map[string]interface{} {
	t.Helper()
	spec, ok := app["spec"].(map[string]interface{})
	require.True(t, ok)
	comps, ok := spec["components"].([]interface{})
	require.True(t, ok)
	out := map[string]map[string]interface{}{}
	for _, item := range comps {
		c, ok := item.(map[string]interface{})
		require.True(t, ok)
		out[c["name"].(string)] = c
	}
	return out
}

func TestResolveAndRenderEmitsInlineModuleAsK8sObjectsComponent(t *testing.T) {
	enableModuleComponentForInlineTest(t)
	app, err := renderWith(t, pkgaddon.InstallPackage{
		AppTemplate:   &v1beta1.Application{},
		YAMLTemplates: []pkgaddon.ElementFile{{Name: "crd.yaml", Data: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: crd-stand-in\n"}},
		InlineModules: []*module.Module{parsedInlineModule(t, "aws-s3")},
	})
	require.NoError(t, err)

	comps := componentsByName(t, app)
	c, ok := comps["aws-s3"]
	require.True(t, ok, "inline module component must be present")
	assert.Equal(t, "k8s-objects", c["type"])
	assert.Contains(t, c["dependsOn"], "inline-addon-resources")

	objs := c["properties"].(map[string]interface{})["objects"].([]interface{})
	require.Len(t, objs, 1)
	owned := objs[0].(map[string]interface{})
	assert.Equal(t, "Application", owned["kind"])
	assert.Equal(t, "module-aws-s3", owned["metadata"].(map[string]interface{})["name"])
	assert.Equal(t, "vela-system", owned["metadata"].(map[string]interface{})["namespace"])
}

func TestResolveAndRenderInlineModuleDependsOnAddonAuxiliaries(t *testing.T) {
	enableModuleComponentForInlineTest(t)
	cueTemplate := `output: {
	apiVersion: "core.oam.dev/v1beta1"
	kind:       "Application"
	metadata: {
		name:      "inline-addon"
		namespace: "vela-system"
	}
	spec: components: []
}
outputs: crossplaneProvider: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "crossplane-provider-stand-in"
		namespace: "vela-system"
	}
}`
	app, err := renderWith(t, pkgaddon.InstallPackage{
		AppCueTemplate: pkgaddon.ElementFile{Data: cueTemplate},
		InlineModules:  []*module.Module{parsedInlineModule(t, "aws-s3")},
	})
	require.NoError(t, err)

	comps := componentsByName(t, app)
	require.Contains(t, comps, "addon-auxiliaries")
	require.Contains(t, comps, "aws-s3")
	assert.Contains(t, comps["aws-s3"]["dependsOn"], "addon-auxiliaries")
}

func TestResolveAndRenderInlineAndImportedModulesCoexist(t *testing.T) {
	enableModuleComponentForInlineTest(t)
	app, err := renderWith(t, pkgaddon.InstallPackage{
		AppTemplate:   &v1beta1.Application{},
		InlineModules: []*module.Module{parsedInlineModule(t, "aws-s3")},
		Imports:       []pkgaddon.ModuleImport{{Module: "aws-efs", Enabled: true, Registry: "oam-modules"}},
	})
	require.NoError(t, err)

	comps := componentsByName(t, app)
	assert.Equal(t, "k8s-objects", comps["aws-s3"]["type"])
	assert.Equal(t, "module", comps["aws-efs"]["type"])
}

func TestResolveAndRenderInlineModuleCollisions(t *testing.T) {
	enableModuleComponentForInlineTest(t)
	handWritten := common2.ApplicationComponent{
		Name:       "aws-s3",
		Type:       "module",
		Properties: &runtime.RawExtension{Raw: []byte(`{}`)},
	}
	tests := map[string]struct {
		pkg  pkgaddon.InstallPackage
		want []string
	}{
		"inline vs import": {
			pkg: pkgaddon.InstallPackage{
				AppTemplate:   &v1beta1.Application{},
				InlineModules: []*module.Module{parsedInlineModule(t, "aws-s3")},
				Imports:       []pkgaddon.ModuleImport{{Module: "aws-s3", Enabled: true}},
			},
			want: []string{`"aws-s3"`, "modules/_imports.cue", "modules/aws-s3"},
		},
		"inline vs hand-written": {
			pkg: pkgaddon.InstallPackage{
				AppTemplate:   &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{handWritten}}},
				InlineModules: []*module.Module{parsedInlineModule(t, "aws-s3")},
			},
			want: []string{`"aws-s3"`, "hand-written", "modules/aws-s3"},
		},
		"hand-written vs import": {
			pkg: pkgaddon.InstallPackage{
				AppTemplate: &v1beta1.Application{Spec: v1beta1.ApplicationSpec{Components: []common2.ApplicationComponent{handWritten}}},
				Imports:     []pkgaddon.ModuleImport{{Module: "aws-s3", Enabled: true}},
			},
			want: []string{`"aws-s3"`, "hand-written", "modules/_imports.cue"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := renderWith(t, tc.pkg)
			require.Error(t, err)
			for _, s := range tc.want {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

func TestResolveAndRenderWithoutInlineModulesAddsNoModuleComponents(t *testing.T) {
	app, err := renderWith(t, pkgaddon.InstallPackage{
		AppTemplate:   &v1beta1.Application{},
		YAMLTemplates: []pkgaddon.ElementFile{{Name: "crd.yaml", Data: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: crd-stand-in\n"}},
	})
	require.NoError(t, err)
	for name, c := range componentsByName(t, app) {
		assert.NotEqual(t, "module", c["type"], name)
		assert.NotContains(t, name, "aws-")
	}
}
