/*
Copyright 2021 The KubeVela Authors.

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
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyMinimalModule returns a fresh minimal module directory so each test case
// can mutate its own private copy without disturbing other cases.
func copyMinimalModule(t *testing.T) string {
	t.Helper()
	return minimalModuleDir(t)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// findByKind returns the first object of kind in objs, or nil if none match.
// Auxiliary objects carry no name convention (that is the point of the
// generalized auxiliary/ read), so tests locate the object they care about by
// its "kind" field instead of assuming it is the only or the first entry.
func findByKind(objs []map[string]interface{}, kind string) map[string]interface{} {
	for _, obj := range objs {
		if k, _ := obj["kind"].(string); k == kind {
			return obj
		}
	}
	return nil
}

func TestParseModule_WellFormedModule(t *testing.T) {
	mod, err := ParseModuleDir(minimalModuleDir(t))
	require.NoError(t, err)
	require.NotNil(t, mod)

	assert.Equal(t, "minimal", mod.Name)
	assert.Equal(t, "1.0.0", mod.Version)

	require.NotEmpty(t, mod.Auxiliary)
	xrd := findByKind(mod.Auxiliary, "CompositeResourceDefinition")
	require.NotNil(t, xrd, "expected a CompositeResourceDefinition among the module's auxiliary objects")
	xrdMetadata, ok := xrd["metadata"].(map[string]interface{})
	require.True(t, ok, "xrd metadata must be a map")
	assert.Equal(t, "xwidgets.example.com", xrdMetadata["name"])

	require.Contains(t, mod.Lines, "v1")
	line := mod.Lines["v1"]
	assert.Equal(t, "v1", line.APIVersion)

	require.NotEmpty(t, line.Auxiliary)
	comp := findByKind(line.Auxiliary, "Composition")
	require.NotNil(t, comp, "expected a Composition among the line's auxiliary objects")
	compMetadata, ok := comp["metadata"].(map[string]interface{})
	require.True(t, ok, "composition metadata must be a map")
	assert.Equal(t, "widgets.example.com", compMetadata["name"])

	require.Len(t, line.Definitions, 1)
	def := line.Definitions[0]
	defMetadata, ok := def["metadata"].(map[string]interface{})
	require.True(t, ok, "definition metadata must be a map")
	assert.Equal(t, "widget", defMetadata["name"])
}

// TestParseModule_StructuralAndIdentityFailures starts from one minimal
// valid module, copied fresh per case, and breaks exactly one thing per
// case — since ParseModule stops at the first error, each fixture only
// needs to differ from a valid module by the single part under test.
func TestParseModule_StructuralAndIdentityFailures(t *testing.T) {
	cases := []struct {
		name          string
		mutate        func(t *testing.T, dir string)
		wantErrSubstr string
	}{
		{
			name: "missing _module.cue",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "_module.cue")))
			},
			wantErrSubstr: "_module.cue",
		},
		{
			name: "missing module field",
			mutate: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_module.cue"), `version: "1.0.0"`+"\n")
			},
			wantErrSubstr: "_module.cue",
		},
		{
			name: "missing version field",
			mutate: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_module.cue"), `module: "minimal"`+"\n")
			},
			wantErrSubstr: "_module.cue",
		},
		{
			name: "non-semver version",
			mutate: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "_module.cue"), "module: \"minimal\"\nversion: \"latest\"\n")
			},
			wantErrSubstr: "_module.cue",
		},
		{
			name: "line missing _version.cue",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "v1", "_version.cue")))
			},
			wantErrSubstr: "_version.cue",
		},
		{
			name: "line with empty definitions/",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "v1", "definitions", "widget.yaml")))
			},
			wantErrSubstr: "definitions",
		},
		{
			name: "module with no API lines",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.RemoveAll(filepath.Join(dir, "v1")))
			},
			wantErrSubstr: "no API lines",
		},
		{
			name: "unsupported definition extension",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "v1", "definitions", "widget.yaml")))
				writeFile(t, filepath.Join(dir, "v1", "definitions", "widget.txt"), "not a definition\n")
			},
			wantErrSubstr: "widget.txt",
		},
		{
			name: "duplicate apiVersion across line directories",
			mutate: func(t *testing.T, dir string) {
				require.NoError(t, os.CopyFS(filepath.Join(dir, "v1x"), os.DirFS(filepath.Join(dir, "v1"))))
			},
			wantErrSubstr: "duplicate line",
		},
		{
			name: "malformed apiVersion",
			mutate: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "v1", "_version.cue"), `apiVersion: "1.0"`+"\n")
			},
			wantErrSubstr: "_version.cue",
		},
		{
			name: "definition with empty metadata.name",
			mutate: func(t *testing.T, dir string) {
				writeFile(t, filepath.Join(dir, "v1", "definitions", "widget.yaml"), "apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  namespace: vela-system\n")
			},
			wantErrSubstr: "widget.yaml",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := copyMinimalModule(t)
			c.mutate(t, dir)

			mod, err := ParseModuleDir(dir)
			require.Error(t, err)
			require.Nil(t, mod)
			assert.Contains(t, err.Error(), c.wantErrSubstr)
		})
	}
}

// TestParseModule_OptionalAuxiliaryResources covers modules that ship no
// Crossplane infra tier: the module-wide XRD and a line's Composition are
// both optional. A module with neither is still valid as long as its
// identity and definitions are well-formed.
func TestParseModule_OptionalAuxiliaryResources(t *testing.T) {
	t.Run("module without an XRD", func(t *testing.T) {
		dir := copyMinimalModule(t)
		require.NoError(t, os.Remove(filepath.Join(dir, "auxiliary", "xrd.yaml")))

		mod, err := ParseModuleDir(dir)
		require.NoError(t, err)
		require.NotNil(t, mod)

		assert.Empty(t, mod.Auxiliary)
		require.Contains(t, mod.Lines, "v1")
		line := mod.Lines["v1"]
		assert.NotEmpty(t, line.Auxiliary)
		require.Len(t, line.Definitions, 1)
	})

	t.Run("line without a Composition", func(t *testing.T) {
		dir := copyMinimalModule(t)
		require.NoError(t, os.Remove(filepath.Join(dir, "v1", "auxiliary", "composition.yaml")))

		mod, err := ParseModuleDir(dir)
		require.NoError(t, err)
		require.NotNil(t, mod)

		assert.NotEmpty(t, mod.Auxiliary)
		require.Contains(t, mod.Lines, "v1")
		line := mod.Lines["v1"]
		assert.Empty(t, line.Auxiliary)
		require.Len(t, line.Definitions, 1)
	})
}

func TestParseModule_FS(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue":                   {Data: []byte(`module: "s3"` + "\n" + `version: "1.0.0"`)},
		"auxiliary/xrd.yaml":            {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: CompositeResourceDefinition\nmetadata:\n  name: xs3\n")},
		"v1/_version.cue":               {Data: []byte(`apiVersion: "v1"`)},
		"v1/auxiliary/composition.yaml": {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: Composition\nmetadata:\n  name: s3\n")},
		"v1/definitions/bucket.yaml":    {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n")},
	}
	mod, err := ParseModule(fsys)
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
	require.Equal(t, "1.0.0", mod.Version)
	require.NotEmpty(t, mod.Auxiliary)
	require.Contains(t, mod.Lines, "v1")
	require.NotEmpty(t, mod.Lines["v1"].Auxiliary)
	require.Len(t, mod.Lines["v1"].Definitions, 1)
}

// TestParseModule_AuxiliaryReadsEveryFile is the core generalization test:
// auxiliary/ is not read by fixed filename (xrd.yaml, composition.yaml).
// Every file in it is read and installed, in filename order, regardless of
// what it is named. This covers both a module shipping more than one
// auxiliary object and a line doing the same.
func TestParseModule_AuxiliaryReadsEveryFile(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue": {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		// Neither file is named "xrd.yaml": both are still read.
		"auxiliary/a-config.yaml": {Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: s3-defaults\n")},
		"auxiliary/b-xrd.yaml":    {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: CompositeResourceDefinition\nmetadata:\n  name: xs3\n")},
		"v1/_version.cue":         {Data: []byte("apiVersion: \"v1\"")},
		// Neither file is named "composition.yaml".
		"v1/auxiliary/extra.yaml":    {Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: s3-v1-extra\n")},
		"v1/auxiliary/the-comp.yaml": {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: Composition\nmetadata:\n  name: s3\n")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.NoError(t, err)

	require.Len(t, mod.Auxiliary, 2, "both module-level auxiliary files are read")
	assert.NotNil(t, findByKind(mod.Auxiliary, "ConfigMap"))
	assert.NotNil(t, findByKind(mod.Auxiliary, "CompositeResourceDefinition"))
	// a-config.yaml sorts before b-xrd.yaml.
	assert.Equal(t, "ConfigMap", mod.Auxiliary[0]["kind"])

	line := mod.Lines["v1"]
	require.Len(t, line.Auxiliary, 2, "both line-level auxiliary files are read")
	assert.NotNil(t, findByKind(line.Auxiliary, "ConfigMap"))
	assert.NotNil(t, findByKind(line.Auxiliary, "Composition"))
}

// TestParseModule_AuxiliaryCUEFile covers a .cue auxiliary file: it is a
// single plain Kubernetes object at its root (apiVersion/kind/metadata
// directly), not wrapped the way definitions/*.cue is.
func TestParseModule_AuxiliaryCUEFile(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue": {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		"auxiliary/xrd.cue": {Data: []byte(`
apiVersion: "apiextensions.crossplane.io/v1"
kind:       "CompositeResourceDefinition"
metadata: name: "xs3"
`)},
		"v1/_version.cue":            {Data: []byte("apiVersion: \"v1\"")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.NoError(t, err)

	require.Len(t, mod.Auxiliary, 1)
	xrd := mod.Auxiliary[0]
	assert.Equal(t, "CompositeResourceDefinition", xrd["kind"])
	meta, ok := xrd["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "xs3", meta["name"])
}

// TestParseModule_AuxiliaryMultiDocumentYAML covers a single auxiliary file
// holding several "---"-separated documents: each becomes its own object,
// the same way a plain Kubernetes manifest can bundle several resources.
func TestParseModule_AuxiliaryMultiDocumentYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue": {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		"auxiliary/bundle.yaml": {Data: []byte(`
apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: xs3
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: s3-defaults
`)},
		"v1/_version.cue":            {Data: []byte("apiVersion: \"v1\"")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.NoError(t, err)

	require.Len(t, mod.Auxiliary, 2, "one file with two documents yields two objects")
	assert.NotNil(t, findByKind(mod.Auxiliary, "CompositeResourceDefinition"))
	assert.NotNil(t, findByKind(mod.Auxiliary, "ConfigMap"))
}

// TestParseModule_AuxiliaryUnsupportedExtension is the failure-mode guard
// this whole generalization exists for: an auxiliary file the parser cannot
// decode is a loud error naming the file, never a silent skip.
func TestParseModule_AuxiliaryUnsupportedExtension(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue":                {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		"auxiliary/xrd.txt":          {Data: []byte("not yaml or cue\n")},
		"v1/_version.cue":            {Data: []byte("apiVersion: \"v1\"")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.Error(t, err)
	require.Nil(t, mod)
	assert.Contains(t, err.Error(), "xrd.txt")
}

func TestParseModule_EnabledDefaultsTrue(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue":                {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		"v1/_version.cue":            {Data: []byte("apiVersion: \"v1\"")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.NoError(t, err)
	require.True(t, mod.Lines["v1"].Enabled, "a line with no enabled field defaults to enabled")
}

func TestParseModule_EnabledFalseIsCaptured(t *testing.T) {
	fsys := fstest.MapFS{
		"_module.cue":                {Data: []byte("module: \"s3\"\nversion: \"1.0.0\"")},
		"v1/_version.cue":            {Data: []byte("apiVersion: \"v1\"\nenabled: false")},
		"v1/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
		"v2/_version.cue":            {Data: []byte("apiVersion: \"v2\"\nenabled: true")},
		"v2/definitions/bucket.yaml": {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: bucket\n")},
	}

	mod, err := ParseModule(fsys)
	require.NoError(t, err)
	require.False(t, mod.Lines["v1"].Enabled)
	require.True(t, mod.Lines["v2"].Enabled)
}
