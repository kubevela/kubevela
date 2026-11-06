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
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

// ownedApplication returns the module-<name> Application wrapped by an inline
// module's k8s-objects component.
func ownedApplication(t *testing.T, pkg *InstallPackage) map[string]interface{} {
	t.Helper()
	comps, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.NoError(t, err)
	require.Len(t, comps, 1)
	var props struct {
		Objects []map[string]interface{} `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(comps[0].Properties.Raw, &props))
	require.Len(t, props.Objects, 1)
	return props.Objects[0]
}

// Decision 2 of the story design: an inline module has no namespace override,
// so every definition it ships installs into vela-system.
func TestRenderInlineModuleComponentsInstallsDefinitionsIntoVelaSystem(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	app := ownedApplication(t, pkg)
	assert.Equal(t, "vela-system", app["metadata"].(map[string]interface{})["namespace"])

	var defsTier map[string]interface{}
	for _, c := range app["spec"].(map[string]interface{})["components"].([]interface{}) {
		if c.(map[string]interface{})["name"] == "aws-s3-v1-defs" {
			defsTier = c.(map[string]interface{})
		}
	}
	require.NotNil(t, defsTier, "the v1 definitions tier must be rendered")
	objs := defsTier["properties"].(map[string]interface{})["objects"].([]interface{})
	require.NotEmpty(t, objs)
	for _, o := range objs {
		meta := o.(map[string]interface{})["metadata"].(map[string]interface{})
		assert.Equal(t, "vela-system", meta["namespace"], "definition %v", meta["name"])
	}
}

func TestReadInlineModulesDirTwoDirectoriesClaimingOneModuleNameCollide(t *testing.T) {
	enableModuleComponent(t)
	files := map[string]string{}
	maps.Copy(files, s3ModuleFiles("modules/first", "dup"))
	maps.Copy(files, s3ModuleFiles("modules/second", "dup"))
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))
	require.Len(t, pkg.InlineModules, 2)

	_, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.Error(t, err, "two inline modules with the same module name must not both render")
	assert.Contains(t, err.Error(), `"dup"`)
}

func TestRenderInlineModuleComponentsDisabledImportDoesNotCollide(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{
		Meta:    Meta{Name: "example"},
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: false}},
	}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.NoError(t, err)
	require.Len(t, comps, 1)
}

func TestRenderInlineModuleComponentsAvoidsNameClashWithNonModuleComponent(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineModuleComponents(pkg, []common2.ApplicationComponent{{Name: "aws-s3", Type: "webservice"}}, nil, "")
	require.NoError(t, err)
	require.Len(t, comps, 1)
	assert.NotEqual(t, "aws-s3", comps[0].Name, "must not reuse a component name the app already has")
}
