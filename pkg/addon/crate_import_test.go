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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
)

func TestParseCrateImportsSingleEnabledImport(t *testing.T) {
	data := `imports: [
    {
        crate:  "aws-s3"
        enabled: true
        sources: [
            { registry: "oam-crates", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseCrateImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Equal(t, CrateImport{
		Crate:    "aws-s3",
		Enabled:  true,
		Registry: "oam-crates",
		Version:  "1.0.0",
	}, imports[0])
}

func TestParseCrateImportsEnabledDefaultsTrueWhenAbsent(t *testing.T) {
	data := `imports: [
    {
        crate: "aws-s3"
        sources: [
            { registry: "oam-crates", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseCrateImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.True(t, imports[0].Enabled)
}

func TestParseCrateImportsExplicitlyDisabled(t *testing.T) {
	data := `imports: [
    {
        crate:  "aws-s3"
        enabled: false
        sources: [
            { registry: "oam-crates", version: "1.0.0" },
        ]
    },
]`
	imports, err := parseCrateImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.False(t, imports[0].Enabled)
}

func TestParseCrateImportsRegistryAndVersionOptional(t *testing.T) {
	data := `imports: [
    {
        crate: "aws-s3"
        sources: [
            {},
        ]
    },
]`
	imports, err := parseCrateImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Empty(t, imports[0].Registry)
	assert.Empty(t, imports[0].Version)
}

func TestParseCrateImportsZeroSourcesIsError(t *testing.T) {
	data := `imports: [
    {
        crate:  "aws-s3"
        enabled: true
        sources: []
    },
]`
	_, err := parseCrateImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "exactly one sources")
}

func TestParseCrateImportsMultipleSourcesIsError(t *testing.T) {
	data := `imports: [
    {
        crate:  "aws-s3"
        enabled: true
        sources: [
            { registry: "oam-crates", version: "1.0.0" },
            { registry: "oam-crates", version: "2.0.0" },
        ]
    },
]`
	_, err := parseCrateImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "exactly one sources")
}

func TestParseCrateImportsMissingCrateNameIsError(t *testing.T) {
	data := `imports: [
    {
        enabled: true
        sources: [
            { registry: "oam-crates", version: "1.0.0" },
        ]
    },
]`
	_, err := parseCrateImports(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "crate")
}

func TestParseCrateImportsNoImportsIsEmpty(t *testing.T) {
	imports, err := parseCrateImports(`imports: []`)
	require.NoError(t, err)
	assert.Empty(t, imports)
}

func TestParseCrateImportsVersionsFieldDoesNotError(t *testing.T) {
	// versions (the API-line filter) is parsed but not enforced this story
	// (see the design doc's scope decisions); it must not fail parsing.
	data := `imports: [
    {
        crate: "aws-s3"
        sources: [
            { registry: "oam-crates", version: "1.0.0", versions: ["v1"] },
        ]
    },
]`
	imports, err := parseCrateImports(data)
	require.NoError(t, err)
	require.Len(t, imports, 1)
	assert.Equal(t, "aws-s3", imports[0].Crate)
}

type fixedFileReader struct {
	data string
	err  error
}

func (f fixedFileReader) ListAddonMeta() (map[string]SourceMeta, error) { return nil, nil }
func (f fixedFileReader) ReadFile(_ string) (string, error)             { return f.data, f.err }
func (f fixedFileReader) RelativePath(item Item) string                 { return item.GetPath() }

func TestReadCrateImportsFilePopulatesInstallPackage(t *testing.T) {
	reader := fixedFileReader{data: `imports: [
    { crate: "aws-s3", enabled: true, sources: [{ registry: "oam-crates", version: "1.0.0" }] },
]`}
	pkg := &InstallPackage{}
	err := readCrateImportsFile(pkg, reader, "crates/_imports.cue")
	require.NoError(t, err)
	require.Len(t, pkg.Imports, 1)
	assert.Equal(t, "aws-s3", pkg.Imports[0].Crate)
}

func TestGetPatternFromItemRecognizesCratesImportsFile(t *testing.T) {
	item := mockItem{path: "my-addon/crates/_imports.cue"}
	pattern := GetPatternFromItem(item, mockReader{}, "my-addon")
	assert.Equal(t, CratesImportsFileName, pattern)
}

func TestRenderCrateComponentsEmitsEnabledImport(t *testing.T) {
	addon := &InstallPackage{
		Meta: Meta{Name: "platform-storage"},
		Imports: []CrateImport{
			{Crate: "aws-s3", Enabled: true, Registry: "oam-crates", Version: "1.0.0"},
		},
	}
	comps, err := RenderCrateComponents(addon, nil, []string{"platform-storage-resources"}, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)

	c := comps[0]
	assert.Equal(t, "aws-s3", c.Name)
	assert.Equal(t, "crate", c.Type)
	assert.Equal(t, []string{"platform-storage-resources"}, c.DependsOn)

	var props map[string]interface{}
	require.NoError(t, json.Unmarshal(c.Properties.Raw, &props))
	assert.Equal(t, "aws-s3", props["crate"])
	assert.Equal(t, "oam-crates", props["registry"])
	assert.Equal(t, "1.0.0", props["version"])
}

func TestRenderCrateComponentsOmitsEmptyRegistryAndVersion(t *testing.T) {
	addon := &InstallPackage{
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: true}},
	}
	comps, err := RenderCrateComponents(addon, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)

	var props map[string]interface{}
	require.NoError(t, json.Unmarshal(comps[0].Properties.Raw, &props))
	assert.Equal(t, map[string]interface{}{"crate": "aws-s3"}, props)
	assert.Nil(t, comps[0].DependsOn)
}

func TestRenderCrateComponentsSkipsDisabledImport(t *testing.T) {
	addon := &InstallPackage{
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: false}},
	}
	comps, err := RenderCrateComponents(addon, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderCrateComponentsNoImportsIsEmpty(t *testing.T) {
	comps, err := RenderCrateComponents(&InstallPackage{}, nil, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderCrateComponentsErrorsWhenCrateAlreadyDeclaredByProperties(t *testing.T) {
	// No source silently wins over another: a hand-written component and an
	// import claiming the same crate name is a packaging mistake, not an
	// intentional override, so this fails the whole render.
	addon := &InstallPackage{
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{
			Name:       "author-declared-aws-s3",
			Type:       "crate",
			Properties: &runtime.RawExtension{Raw: []byte(`{"crate":"aws-s3"}`)},
		},
	}
	_, err := RenderCrateComponents(addon, existing, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "author-declared-aws-s3")
}

func TestRenderCrateComponentsErrorsWhenCrateAlreadyDeclaredByComponentName(t *testing.T) {
	// The type: crate component template defaults properties.crate to the
	// component's own name (crate: *context.name | string), so a
	// hand-authored component with no explicit "crate" property must still
	// be matched by its name.
	addon := &InstallPackage{
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{Name: "aws-s3", Type: "crate"},
	}
	_, err := RenderCrateComponents(addon, existing, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
}

func TestRenderCrateComponentsDoesNotSkipOnNonCrateComponentNameCollision(t *testing.T) {
	addon := &InstallPackage{
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: true}},
	}
	existing := []common2.ApplicationComponent{
		{Name: "aws-s3", Type: "k8s-objects"},
	}
	comps, err := RenderCrateComponents(addon, existing, nil, nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)
}

func TestRenderCrateComponentsErrorsOnRepeatedCrateName(t *testing.T) {
	// Two imports entries naming the same crate is also a collision: nothing
	// picks a winner, including between two entries of the same kind.
	addon := &InstallPackage{
		Imports: []CrateImport{
			{Crate: "aws-s3", Enabled: true, Registry: "oam-crates", Version: "1.0.0"},
			{Crate: "aws-s3", Enabled: true, Registry: "oam-crates", Version: "2.0.0"},
		},
	}
	_, err := RenderCrateComponents(addon, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
}
