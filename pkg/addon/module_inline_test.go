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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"

	common2 "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/module"
	modulerender "github.com/oam-dev/kubevela/pkg/module/service"
)

// enableModuleComponent turns EnableModuleComponent on for tests that exercise
// the inline-render happy path; RenderInlineModuleComponents refuses outright
// when the gate is off (mirroring the external path's own CueX gate check).
func enableModuleComponent(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableModuleComponent, true)
}

// mapFileReader is a minimal AsyncReader backed by a path->content map, so a
// test can simulate several files belonging to one addon tree. Unlike
// fixedFileReader (module_import_test.go), it returns different content per
// path, which an inline module's multi-file tree needs.
type mapFileReader struct {
	files map[string]string
}

func (r mapFileReader) ListAddonMeta() (map[string]SourceMeta, error) { return nil, nil }
func (r mapFileReader) RelativePath(item Item) string                 { return item.GetPath() }
func (r mapFileReader) ReadFile(path string) (string, error) {
	data, ok := r.files[path]
	if !ok {
		return "", os.ErrNotExist
	}
	return data, nil
}

// itemsFor turns a path->content map into the Item list readInlineModulesDir
// expects, in the shape ClassifyItemByPattern would have produced.
func itemsFor(files map[string]string) []Item {
	items := make([]Item, 0, len(files))
	for p := range files {
		items = append(items, mockItem{path: p})
	}
	return items
}

// s3ModuleFiles is a minimal, valid inline module tree: one enabled v1 line,
// one ConfigMap-backed definition (no Crossplane dependency), no auxiliary/
// at either scope. moduleName controls _module.cue's own "module" field so
// the same fixture can be reused under different addon paths without a
// name collision between test cases.
func s3ModuleFiles(root, moduleName string) map[string]string {
	return map[string]string{
		root + "/_module.cue":                `module: "` + moduleName + `"` + "\nversion: \"0.1.0\"",
		root + "/v1/_version.cue":            `apiVersion: "v1"`,
		root + "/v1/definitions/bucket.yaml": s3BucketDefinitionYAML,
	}
}

const s3BucketDefinitionYAML = `apiVersion: core.oam.dev/v1beta1
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

func TestReadInlineModulesDirDetectsOneModule(t *testing.T) {
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	require.Len(t, pkg.InlineModules, 1)
	assert.Equal(t, "aws-s3", pkg.InlineModules[0].Name)
	assert.Contains(t, pkg.InlineModules[0].Lines, "v1")
}

func TestReadInlineModulesDirDetectsMultipleModules(t *testing.T) {
	files := map[string]string{}
	for k, v := range s3ModuleFiles("modules/aws-s3", "aws-s3") {
		files[k] = v
	}
	for k, v := range s3ModuleFiles("modules/aws-efs", "aws-efs") {
		files[k] = v
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	require.Len(t, pkg.InlineModules, 2)
	names := map[string]bool{}
	for _, mod := range pkg.InlineModules {
		names[mod.Name] = true
	}
	assert.True(t, names["aws-s3"])
	assert.True(t, names["aws-efs"])
}

func TestReadInlineModulesDirErrorsOnDirectoryWithNoModuleCue(t *testing.T) {
	// modules/docs/ ships a file but never a _module.cue at its root. Every
	// subdirectory under modules/ is expected to be an inline module, so this
	// fails loudly instead of being silently dropped.
	files := map[string]string{
		"modules/docs/README.md": "not a module",
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modules/docs")
	assert.Contains(t, err.Error(), "_module.cue")
	assert.Empty(t, pkg.InlineModules)
}

func TestReadInlineModulesDirErrorsOnModuleContentWithNoModuleCue(t *testing.T) {
	// A directory with a real line, _version.cue, and definitions, but no
	// _module.cue -- a forgotten or misnamed file, not an unrelated folder.
	// This must fail loudly: the external path (ParseModuleDir) would fail the
	// same way on this exact tree, and a silent drop here would install the
	// addon with the module just missing, no error anywhere.
	files := map[string]string{
		"modules/aws-s3/v1/_version.cue":            `apiVersion: "v1"`,
		"modules/aws-s3/v1/definitions/bucket.yaml": s3BucketDefinitionYAML,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "modules/aws-s3")
	assert.Contains(t, err.Error(), "_module.cue")
	assert.Empty(t, pkg.InlineModules)
}

func TestReadInlineModulesDirPropagatesParseError(t *testing.T) {
	files := map[string]string{
		"modules/aws-s3/_module.cue":                `module: "aws-s3"` + "\nversion: \"1.0\"", // invalid semver-ish version is fine; apiVersion below is what's invalid
		"modules/aws-s3/v1/_version.cue":            `apiVersion: "1.0"`,
		"modules/aws-s3/v1/definitions/bucket.yaml": s3BucketDefinitionYAML,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aws-s3")
}

func TestReadInlineModulesDirDoesNotMisreadModulesImportsFile(t *testing.T) {
	// modules/_imports.cue has no subdirectory under modules/, so it never
	// forms a module-name group; readModuleImportsFile (its own, more
	// specific pattern) is what reads it, not this function.
	files := map[string]string{
		ModulesImportsFileName: `imports: []`,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineModulesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	assert.Empty(t, pkg.InlineModules)
}

func TestRenderInlineModuleComponentsNoInlineModulesIsEmpty(t *testing.T) {
	comps, err := RenderInlineModuleComponents(&InstallPackage{}, nil, nil, "")
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderInlineModuleComponentsRefusesWhenGateOff(t *testing.T) {
	// Feature gates default to false. An inline module has no CueX render to
	// gate the way type: module's does (pkg/cue/cuex/providers/module/module.go),
	// so without its own check it would install regardless of
	// EnableModuleComponent -- this pins that it does not.
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	_, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableModuleComponent")
}

func TestRenderInlineModuleComponentsEmitsK8sObjectsComponent(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineModuleComponents(pkg, nil, []string{"example-resources"}, "")
	require.NoError(t, err)
	require.Len(t, comps, 1)

	c := comps[0]
	assert.Equal(t, "aws-s3", c.Name)
	assert.Equal(t, "k8s-objects", c.Type)
	assert.Equal(t, []string{"example-resources"}, c.DependsOn)

	var props struct {
		Objects []map[string]interface{} `json:"objects"`
	}
	require.NoError(t, json.Unmarshal(c.Properties.Raw, &props))
	require.Len(t, props.Objects, 1)
	app := props.Objects[0]
	assert.Equal(t, "Application", app["kind"])
	meta := app["metadata"].(map[string]interface{})
	assert.Equal(t, "module-aws-s3", meta["name"])
}

func TestRenderInlineModuleComponentsMatchesExternalPathForSameSource(t *testing.T) {
	// The inline path (ParseModule over an in-memory tree + RenderApplication)
	// and the external path (ParseModuleDir over the same files on disk +
	// RenderApplication) must produce byte-identical owned Applications for
	// the same module source -- this is the story's "byte-for-byte
	// equivalent" acceptance criterion, verified directly rather than assumed
	// from shared code.
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")

	// Inline path.
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))
	require.Len(t, pkg.InlineModules, 1)
	inlineApp, err := modulerender.RenderApplication(pkg.InlineModules[0], "")
	require.NoError(t, err)

	// External/disk path: the same file contents, written to a real directory.
	dir := t.TempDir()
	for relPath, content := range files {
		rel := relPath[len("modules/aws-s3/"):]
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	mod, err := module.ParseModuleDir(dir)
	require.NoError(t, err)
	externalApp, err := modulerender.RenderApplication(mod, "")
	require.NoError(t, err)

	assert.Equal(t, externalApp, inlineApp)
}

func TestRenderInlineModuleComponentsOrdersAfterResources(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineModuleComponents(pkg, nil, []string{"example-resources", "example-config"}, "")
	require.NoError(t, err)
	require.Len(t, comps, 1)
	assert.Equal(t, []string{"example-resources", "example-config"}, comps[0].DependsOn)
}

func TestRenderInlineModuleComponentsErrorsOnCollisionWithImport(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{
		Meta:    Meta{Name: "example"},
		Imports: []ModuleImport{{Module: "aws-s3", Enabled: true}},
	}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	_, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "modules/_imports.cue")
	assert.Contains(t, err.Error(), "modules/aws-s3")
}

func TestRenderInlineModuleComponentsErrorsOnCollisionWithHandWrittenComponent(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	existing := []common2.ApplicationComponent{
		{
			Name:       "author-declared-aws-s3",
			Type:       "module",
			Properties: &runtime.RawExtension{Raw: []byte(`{"module":"aws-s3"}`)},
		},
	}
	_, err := RenderInlineModuleComponents(pkg, existing, nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
}

func TestMixedAddonRendersBothInlineAndImportedModules(t *testing.T) {
	enableModuleComponent(t)
	files := s3ModuleFiles("modules/aws-s3", "aws-s3")
	pkg := &InstallPackage{
		Meta:    Meta{Name: "example"},
		Imports: []ModuleImport{{Module: "aws-efs", Enabled: true, Registry: "oam-modules"}},
	}
	require.NoError(t, readInlineModulesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	importComps, err := RenderModuleComponents(pkg, nil, nil)
	require.NoError(t, err)
	require.Len(t, importComps, 1)
	assert.Equal(t, "aws-efs", importComps[0].Name)

	inlineComps, err := RenderInlineModuleComponents(pkg, nil, nil, "")
	require.NoError(t, err)
	require.Len(t, inlineComps, 1)
	assert.Equal(t, "aws-s3", inlineComps[0].Name)
}
