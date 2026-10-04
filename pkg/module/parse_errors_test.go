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
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalModuleFS is the in-memory twin of minimalModuleDir: one module-wide
// auxiliary, one line with an auxiliary and one YAML definition.
func minimalModuleFS() fstest.MapFS {
	return fstest.MapFS{
		"_module.cue":                   {Data: []byte("module:  \"minimal\"\nversion: \"1.0.0\"\n")},
		"auxiliary/xrd.yaml":            {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: CompositeResourceDefinition\nmetadata:\n  name: xwidgets.example.com\n")},
		"v1/_version.cue":               {Data: []byte("apiVersion: \"v1\"\n")},
		"v1/auxiliary/composition.yaml": {Data: []byte("apiVersion: apiextensions.crossplane.io/v1\nkind: Composition\nmetadata:\n  name: widgets.example.com\n")},
		"v1/definitions/widget.yaml":    {Data: []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: widget\n")},
	}
}

// faultFS wraps a filesystem and fails the named reads, so the parser's
// error paths that no file content can reach (a directory that cannot be
// listed, a file that cannot be read) are reachable.
type faultFS struct {
	fs.FS
	readDirErr  map[string]error
	readFileErr map[string]error
}

func (f faultFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := f.readDirErr[name]; err != nil {
		return nil, err
	}
	return fs.ReadDir(f.FS, name)
}

func (f faultFS) ReadFile(name string) ([]byte, error) {
	if err := f.readFileErr[name]; err != nil {
		return nil, err
	}
	return fs.ReadFile(f.FS, name)
}

func TestParseModuleRejectsAnInvalidModuleName(t *testing.T) {
	m := minimalModuleFS()
	m["_module.cue"] = &fstest.MapFile{Data: []byte("module: \"Widget_X\"\nversion: \"1.0.0\"\n")}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `parse module: module name "Widget_X" in _module.cue is invalid`)
}

func TestParseModuleRejectsAnUnparsableIdentityFile(t *testing.T) {
	m := minimalModuleFS()
	m["_module.cue"] = &fstest.MapFile{Data: []byte("module: \"minimal\"\nversion: [unterminated\n")}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse module: read module name")
}

func TestParseModuleReportsUnreadableDirectories(t *testing.T) {
	boom := errors.New("disk on fire")

	_, err := ParseModule(faultFS{FS: minimalModuleFS(), readDirErr: map[string]error{".": boom}})
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "parse module: read root")

	// A missing auxiliary directory means "no auxiliary objects"; any other
	// failure to list it is reported.
	_, err = ParseModule(faultFS{FS: minimalModuleFS(), readDirErr: map[string]error{"auxiliary": boom}})
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "parse module: read auxiliary")

	_, err = ParseModule(faultFS{FS: minimalModuleFS(), readDirErr: map[string]error{"v1/auxiliary": boom}})
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "parse line v1: read auxiliary")
}

func TestParseModuleReportsUnreadableFiles(t *testing.T) {
	boom := errors.New("blob gone")

	_, err := ParseModule(faultFS{FS: minimalModuleFS(), readFileErr: map[string]error{"auxiliary/xrd.yaml": boom}})
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "read auxiliary/xrd.yaml")

	_, err = ParseModule(faultFS{FS: minimalModuleFS(), readFileErr: map[string]error{"v1/definitions/widget.yaml": boom}})
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "parse line v1: read definition widget.yaml")
}

func TestParseModuleRejectsANonBooleanEnabledSwitch(t *testing.T) {
	m := minimalModuleFS()
	m["v1/_version.cue"] = &fstest.MapFile{Data: []byte("apiVersion: \"v1\"\nenabled: \"yes\"\n")}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse line v1: read enabled")
}

func TestParseModuleNeedsADefinitionsDirectory(t *testing.T) {
	m := minimalModuleFS()
	delete(m, "v1/definitions/widget.yaml")
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse line v1: read definitions dir")
}

func TestParseModuleRendersCUEDefinitions(t *testing.T) {
	m := minimalModuleFS()
	delete(m, "v1/definitions/widget.yaml")
	m["v1/definitions/gauge.cue"] = &fstest.MapFile{Data: []byte(`
"gauge": {
	type: "component"
	description: "A gauge rendered from vela definition CUE"
	attributes: workload: definition: {
		apiVersion: "v1"
		kind:       "ConfigMap"
	}
}
template: {
	output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: context.name
	}
	parameter: {}
}
`)}
	mod, err := ParseModule(m)
	require.NoError(t, err)
	defs := mod.Lines["v1"].Definitions
	require.Len(t, defs, 1)
	assert.Equal(t, "ComponentDefinition", defs[0]["kind"])
	assert.Equal(t, "gauge", defs[0]["metadata"].(map[string]interface{})["name"])

	m["v1/definitions/gauge.cue"] = &fstest.MapFile{Data: []byte("gauge: {{ not cue")}
	_, err = ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse line v1: render definition gauge.cue")
}

func TestParseModuleRejectsOtherDefinitionFileTypes(t *testing.T) {
	m := minimalModuleFS()
	m["v1/definitions/widget.json"] = &fstest.MapFile{Data: []byte(`{"kind": "ComponentDefinition"}`)}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported definition file type: widget.json")

	m = minimalModuleFS()
	m["v1/definitions/widget.yaml"] = &fstest.MapFile{Data: []byte(": not yaml")}
	_, err = ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse line v1: render definition widget.yaml")
}

func TestParseModuleAuxiliaryCUEMustBeAnObject(t *testing.T) {
	m := minimalModuleFS()
	m["auxiliary/bad.cue"] = &fstest.MapFile{Data: []byte("apiVersion: [unterminated")}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode auxiliary/bad.cue")

	m = minimalModuleFS()
	m["auxiliary/scalar.cue"] = &fstest.MapFile{Data: []byte(`"just a string"`)}
	_, err = ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode cue value", "a CUE file that is not a struct is not a Kubernetes object")
}

func TestParseModuleAuxiliaryYAML(t *testing.T) {
	m := minimalModuleFS()
	m["auxiliary/bad.yaml"] = &fstest.MapFile{Data: []byte("a: [")}
	_, err := ParseModule(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode auxiliary/bad.yaml")

	m = minimalModuleFS()
	m["auxiliary/xrd.yaml"] = &fstest.MapFile{Data: []byte("---\n{}\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: only\n---\n")}
	mod, err := ParseModule(m)
	require.NoError(t, err)
	require.Len(t, mod.Auxiliary, 1, "empty documents are skipped, not turned into empty objects")
	assert.Equal(t, "ConfigMap", mod.Auxiliary[0]["kind"])
}
