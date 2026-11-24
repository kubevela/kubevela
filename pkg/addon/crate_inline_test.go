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
	"github.com/oam-dev/kubevela/pkg/crate"
	craterender "github.com/oam-dev/kubevela/pkg/crate/service"
	"github.com/oam-dev/kubevela/pkg/features"
)

// enableCrateComponent turns EnableCrateComponent on for tests that exercise
// the inline-render happy path; RenderInlineCrateComponents refuses outright
// when the gate is off (mirroring the external path's own CueX gate check).
func enableCrateComponent(t *testing.T) {
	t.Helper()
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultMutableFeatureGate,
		features.EnableCrateComponent, true)
}

// mapFileReader is a minimal AsyncReader backed by a path->content map, so a
// test can simulate several files belonging to one addon tree. Unlike
// fixedFileReader (crate_import_test.go), it returns different content per
// path, which an inline crate's multi-file tree needs.
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

// itemsFor turns a path->content map into the Item list readInlineCratesDir
// expects, in the shape ClassifyItemByPattern would have produced.
func itemsFor(files map[string]string) []Item {
	items := make([]Item, 0, len(files))
	for p := range files {
		items = append(items, mockItem{path: p})
	}
	return items
}

// s3CrateFiles is a minimal, valid inline crate tree: one enabled v1 line,
// one ConfigMap-backed definition (no Crossplane dependency), no auxiliary/
// at either scope. crateName controls _crate.cue's own "crate" field so
// the same fixture can be reused under different addon paths without a
// name collision between test cases.
func s3CrateFiles(root, crateName string) map[string]string {
	return map[string]string{
		root + "/_crate.cue":                 `crate: "` + crateName + `"` + "\nversion: \"0.1.0\"",
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

func TestReadInlineCratesDirDetectsOneCrate(t *testing.T) {
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	require.Len(t, pkg.InlineCrates, 1)
	assert.Equal(t, "aws-s3", pkg.InlineCrates[0].Name)
	assert.Contains(t, pkg.InlineCrates[0].Lines, "v1")
}

func TestReadInlineCratesDirDetectsMultipleCrates(t *testing.T) {
	files := map[string]string{}
	for k, v := range s3CrateFiles("crates/aws-s3", "aws-s3") {
		files[k] = v
	}
	for k, v := range s3CrateFiles("crates/aws-efs", "aws-efs") {
		files[k] = v
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	require.Len(t, pkg.InlineCrates, 2)
	names := map[string]bool{}
	for _, inlineCrate := range pkg.InlineCrates {
		names[inlineCrate.Name] = true
	}
	assert.True(t, names["aws-s3"])
	assert.True(t, names["aws-efs"])
}

func TestReadInlineCratesDirErrorsOnDirectoryWithNoCrateCue(t *testing.T) {
	// crates/docs/ ships a file but never a _crate.cue at its root. Every
	// subdirectory under crates/ is expected to be an inline crate, so this
	// fails loudly instead of being silently dropped.
	files := map[string]string{
		"crates/docs/README.md": "not a crate",
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "crates/docs")
	assert.Contains(t, err.Error(), "_crate.cue")
	assert.Empty(t, pkg.InlineCrates)
}

func TestReadInlineCratesDirErrorsOnCrateContentWithNoCrateCue(t *testing.T) {
	// A directory with a real line, _version.cue, and definitions, but no
	// _crate.cue -- a forgotten or misnamed file, not an unrelated folder.
	// This must fail loudly: the external path (ParseCrateDir) would fail the
	// same way on this exact tree, and a silent drop here would install the
	// addon with the crate just missing, no error anywhere.
	files := map[string]string{
		"crates/aws-s3/v1/_version.cue":            `apiVersion: "v1"`,
		"crates/aws-s3/v1/definitions/bucket.yaml": s3BucketDefinitionYAML,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "crates/aws-s3")
	assert.Contains(t, err.Error(), "_crate.cue")
	assert.Empty(t, pkg.InlineCrates)
}

func TestReadInlineCratesDirPropagatesParseError(t *testing.T) {
	files := map[string]string{
		"crates/aws-s3/_crate.cue":                 `crate: "aws-s3"` + "\nversion: \"1.0\"", // invalid semver-ish version is fine; apiVersion below is what's invalid
		"crates/aws-s3/v1/_version.cue":            `apiVersion: "1.0"`,
		"crates/aws-s3/v1/definitions/bucket.yaml": s3BucketDefinitionYAML,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aws-s3")
}

func TestReadInlineCratesDirDoesNotMisreadCratesImportsFile(t *testing.T) {
	// crates/_imports.cue has no subdirectory under crates/, so it never
	// forms a crate-name group; readCrateImportsFile (its own, more
	// specific pattern) is what reads it, not this function.
	files := map[string]string{
		CratesImportsFileName: `imports: []`,
	}
	reader := mapFileReader{files: files}
	pkg := &InstallPackage{}

	err := readInlineCratesDir(pkg, reader, itemsFor(files), "")
	require.NoError(t, err)
	assert.Empty(t, pkg.InlineCrates)
}

func TestRenderInlineCrateComponentsNoInlineCratesIsEmpty(t *testing.T) {
	comps, err := RenderInlineCrateComponents(&InstallPackage{}, nil, nil, "", nil)
	require.NoError(t, err)
	assert.Empty(t, comps)
}

func TestRenderInlineCrateComponentsRefusesWhenGateOff(t *testing.T) {
	// Feature gates default to false. An inline crate has no CueX render to
	// gate the way type: crate's does (pkg/cue/cuex/providers/crate/crate.go),
	// so without its own check it would install regardless of
	// EnableCrateComponent -- this pins that it does not.
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	_, err := RenderInlineCrateComponents(pkg, nil, nil, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EnableCrateComponent")
}

func TestRenderInlineCrateComponentsEmitsK8sObjectsComponent(t *testing.T) {
	enableCrateComponent(t)
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineCrateComponents(pkg, nil, []string{"example-resources"}, "", nil)
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
	assert.Equal(t, "crate-aws-s3", meta["name"])
}

func TestRenderInlineCrateComponentsMatchesExternalPathForSameSource(t *testing.T) {
	// The inline path (ParseCrate over an in-memory tree + RenderApplication)
	// and the external path (ParseCrateDir over the same files on disk +
	// RenderApplication) must produce byte-identical owned Applications for
	// the same crate source -- this is the story's "byte-for-byte
	// equivalent" acceptance criterion, verified directly rather than assumed
	// from shared code.
	files := s3CrateFiles("crates/aws-s3", "aws-s3")

	// Inline path.
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))
	require.Len(t, pkg.InlineCrates, 1)
	inlineApp, err := craterender.RenderApplication(pkg.InlineCrates[0], "", nil)
	require.NoError(t, err)

	// External/disk path: the same file contents, written to a real directory.
	dir := t.TempDir()
	for relPath, content := range files {
		rel := relPath[len("crates/aws-s3/"):]
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	parsed, err := crate.ParseCrateDir(dir)
	require.NoError(t, err)
	externalApp, err := craterender.RenderApplication(parsed, "", nil)
	require.NoError(t, err)

	assert.Equal(t, externalApp, inlineApp)
}

func TestRenderInlineCrateComponentsOrdersAfterResources(t *testing.T) {
	enableCrateComponent(t)
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	comps, err := RenderInlineCrateComponents(pkg, nil, []string{"example-resources", "example-config"}, "", nil)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	assert.Equal(t, []string{"example-resources", "example-config"}, comps[0].DependsOn)
}

func TestRenderInlineCrateComponentsErrorsOnCollisionWithImport(t *testing.T) {
	enableCrateComponent(t)
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{
		Meta:    Meta{Name: "example"},
		Imports: []CrateImport{{Crate: "aws-s3", Enabled: true}},
	}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	_, err := RenderInlineCrateComponents(pkg, nil, nil, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
	assert.Contains(t, err.Error(), "crates/_imports.cue")
	assert.Contains(t, err.Error(), "crates/aws-s3")
}

func TestRenderInlineCrateComponentsErrorsOnCollisionWithHandWrittenComponent(t *testing.T) {
	enableCrateComponent(t)
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{Meta: Meta{Name: "example"}}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	existing := []common2.ApplicationComponent{
		{
			Name:       "author-declared-aws-s3",
			Type:       "crate",
			Properties: &runtime.RawExtension{Raw: []byte(`{"crate":"aws-s3"}`)},
		},
	}
	_, err := RenderInlineCrateComponents(pkg, existing, nil, "", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"aws-s3"`)
}

func TestMixedAddonRendersBothInlineAndImportedCrates(t *testing.T) {
	enableCrateComponent(t)
	files := s3CrateFiles("crates/aws-s3", "aws-s3")
	pkg := &InstallPackage{
		Meta:    Meta{Name: "example"},
		Imports: []CrateImport{{Crate: "aws-efs", Enabled: true, Registry: "oam-crates"}},
	}
	require.NoError(t, readInlineCratesDir(pkg, mapFileReader{files: files}, itemsFor(files), ""))

	importComps, err := RenderCrateComponents(pkg, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, importComps, 1)
	assert.Equal(t, "aws-efs", importComps[0].Name)

	inlineComps, err := RenderInlineCrateComponents(pkg, nil, nil, "", nil)
	require.NoError(t, err)
	require.Len(t, inlineComps, 1)
	assert.Equal(t, "aws-s3", inlineComps[0].Name)
}
