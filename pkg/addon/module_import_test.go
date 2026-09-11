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
	"testing"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestParseModuleImportsSingleEnabledImport(t *testing.T) {
	data := `imports: [
    {
        module:  "aws-s3"
        enabled: true
        sources: [
            { registry: "oam-modules", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseModuleImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Equal(t, ModuleImport{
		Module:   "aws-s3",
		Enabled:  true,
		Registry: "oam-modules",
		Version:  "1.0.0",
	}, imports[0])
}

func TestParseModuleImportsEnabledDefaultsTrueWhenAbsent(t *testing.T) {
	data := `imports: [
    {
        module: "aws-s3"
        sources: [
            { registry: "oam-modules", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseModuleImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.True(t, imports[0].Enabled)
}

func TestParseModuleImportsExplicitlyDisabled(t *testing.T) {
	data := `imports: [
    {
        module:  "aws-s3"
        enabled: false
        sources: [
            { registry: "oam-modules", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseModuleImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.False(t, imports[0].Enabled)
}

func TestParseModuleImportsRegistryAndVersionOptional(t *testing.T) {
	data := `imports: [
    {
        module: "aws-s3"
        sources: [
            {},
        ]
    },
]`
	imports, err := parseModuleImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Empty(t, imports[0].Registry)
	assert.Empty(t, imports[0].Version)
}

func TestParseModuleImportsZeroSourcesIsError(t *testing.T) {
	data := `imports: [
    {
        module:  "aws-s3"
        enabled: true
        sources: []
    },
]`
	_, err := parseModuleImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "exactly one sources")
}

func TestParseModuleImportsMultipleSourcesIsError(t *testing.T) {
	data := `imports: [
    {
        module:  "aws-s3"
        enabled: true
        sources: [
            { registry: "oam-modules", version: "1.0.0" },
            { registry: "oam-modules", version: "2.0.0" },
        ]
    },
]`
	_, err := parseModuleImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "exactly one sources")
}

func TestParseModuleImportsMissingModuleNameIsError(t *testing.T) {
	data := `imports: [
    {
        enabled: true
        sources: [
            { registry: "oam-modules", version: "1.0.0" },
        ]
    },
]`
	_, err := parseModuleImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module")
}

func TestParseModuleImportsNoImportsIsEmpty(t *testing.T) {
	imports, err := parseModuleImports(`imports: []`)
	require.NoError(t, err)
	assert.Empty(t, imports)
}

func TestParseModuleImportsVersionsFieldDoesNotError(t *testing.T) {
	// versions (the API-line filter) is parsed but not enforced this story
	// (see the design doc's scope decisions); it must not fail parsing.
	data := `imports: [
    {
        module: "aws-s3"
        sources: [
            { registry: "oam-modules", version: "1.0.0", versions: ["v1"] },
        ]
    },
]`
	imports, err := parseModuleImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Equal(t, "aws-s3", imports[0].Module)
}

type fixedFileReader struct {
	data string
	err  error
}

func (f fixedFileReader) ListAddonMeta() (map[string]SourceMeta, error) { return nil, nil }
func (f fixedFileReader) ReadFile(_ string) (string, error)             { return f.data, f.err }
func (f fixedFileReader) RelativePath(item Item) string                 { return item.GetPath() }

func TestReadModuleImportsFilePopulatesInstallPackage(t *testing.T) {
	reader := fixedFileReader{data: `imports: [
    { module: "aws-s3", enabled: true, sources: [{ registry: "oam-modules", version: "1.0.0" }] },
]`}
	pkg := &InstallPackage{}
	err := readModuleImportsFile(pkg, reader, "modules/_imports.cue")
	require.NoError(t, err)
	require.Len(t, pkg.Imports, 1)
	assert.Equal(t, "aws-s3", pkg.Imports[0].Module)
}

func TestGetPatternFromItemRecognizesModulesImportsFile(t *testing.T) {
	item := mockItem{path: "my-addon/modules/_imports.cue"}
	pattern := GetPatternFromItem(item, mockReader{}, "my-addon")
	assert.Equal(t, ModulesImportsFileName, pattern)
}

func TestRenderModuleComponentsEmitsEnabledImport(t *testing.T) {
	addon := &InstallPackage{
		Meta: Meta{Name: "atmos-platform-storage"},
		Imports: []ModuleImport{
			{Module: "aws-s3", Enabled: true, Registry: "oam-modules", Version: "1.0.0"},
		},
	}
	comps, err := RenderModuleComponents(addon, nil, []string{"atmos-platform-storage-resources"})
	require.NoError(t, err)
	require.Len(t, comps, 1)

	c := comps[0]
	assert.Equal(t, "aws-s3", c.Name)
	assert.Equal(t, "module", c.Type)
	assert.Equal(t, []string{"atmos-platform-storage-resources"}, c.DependsOn)

	var props map[string]interface{}
	require.NoError(t, json.Unmarshal(c.Properties.Raw, &props))
	assert.Equal(t, "aws-s3", props["module"])
	assert.Equal(t, "oam-modules", props["registry"])
	assert.Equal(t, "1.0.0", props["version"])
}

func TestRenderModuleComponentsOmitsEmptyRegistryAndVersion(t *testing.T) {
	addon := &InstallPackage{
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: true}},
	}
	comps, err := RenderModuleComponents(addon, nil, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)

	var props map[string]interface{}
	require.NoError(t, json.Unmarshal(comps[0].Properties.Raw, &props))
	assert.Equal(t, map[string]interface{}{"module": "aws-s3"}, props)
	assert.Nil(t, comps[0].DependsOn)
}

func TestRenderModuleComponentsSkipsDisabledImport(t *testing.T) {
	addon := &InstallPackage{
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: false}},
	}
	comps, err := RenderModuleComponents(addon, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderModuleComponentsNoImportsIsEmpty(t *testing.T) {
	comps, err := RenderModuleComponents(&InstallPackage{}, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderModuleComponentsSkipsModuleAlreadyDeclaredByProperties(t *testing.T) {
	addon := &InstallPackage{
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{
			Name:       "author-declared-aws-s3",
			Type:       "module",
			Properties: &runtime.RawExtension{Raw: []byte(`{"module":"aws-s3"}`)},
		},
	}
	comps, err := RenderModuleComponents(addon, existing, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderModuleComponentsSkipsModuleAlreadyDeclaredByComponentName(t *testing.T) {
	// The type: module component template defaults properties.module to the
	// component's own name (module: *context.name | string), so a
	// hand-authored component with no explicit "module" property must still
	// be matched by its name.
	addon := &InstallPackage{
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{Name: "aws-s3", Type: "module"},
	}
	comps, err := RenderModuleComponents(addon, existing, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderModuleComponentsDoesNotSkipOnNonModuleComponentNameCollision(t *testing.T) {
	addon := &InstallPackage{
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{Name: "aws-s3", Type: "k8s-objects"},
	}
	comps, err := RenderModuleComponents(addon, existing, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)
}

func TestRenderModuleComponentsDedupesRepeatedModuleName(t *testing.T) {
	// Two imports entries naming the same module must not both be emitted --
	// the second occurrence is treated the same as an author-declared
	// component that already claimed the name.
	addon := &InstallPackage{
		Imports: []ModuleImport{
			{Module: "aws-s3", Enabled: true, Registry: "oam-modules", Version: "1.0.0"},
			{Module: "aws-s3", Enabled: true, Registry: "oam-modules", Version: "2.0.0"},
		},
	}
	comps, err := RenderModuleComponents(addon, nil, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	assert.Equal(t, "aws-s3", comps[0].Name)
}
