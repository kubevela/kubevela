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

package service

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/module"
)

func TestMapFS_ReadFileAndReadDir(t *testing.T) {
	m := mapFS{
		"_module.cue":               []byte(`module: "s3"`),
		"v1/_version.cue":           []byte(`apiVersion: "v1"`),
		"v1/definitions/bucket.cue": []byte("x"),
	}

	// ReadFile
	data, err := fs.ReadFile(m, "v1/_version.cue")
	require.NoError(t, err)
	require.Equal(t, `apiVersion: "v1"`, string(data))

	// Missing file -> fs.ErrNotExist
	_, err = fs.ReadFile(m, "nope.cue")
	require.ErrorIs(t, err, fs.ErrNotExist)

	// ReadDir root: one file (_module.cue) + one synthesized dir (v1)
	root, err := fs.ReadDir(m, ".")
	require.NoError(t, err)
	names := map[string]bool{}
	for _, e := range root {
		names[e.Name()] = e.IsDir()
	}
	require.False(t, names["_module.cue"])
	require.True(t, names["v1"])

	// ReadDir nested
	defs, err := fs.ReadDir(m, "v1/definitions")
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, "bucket.cue", defs[0].Name())

	// ReadDir of a path with no files -> fs.ErrNotExist
	_, err = fs.ReadDir(m, "v2")
	require.ErrorIs(t, err, fs.ErrNotExist)
}

func TestMapFS_ParsesThroughParser(t *testing.T) {
	m := mapFS{
		"_module.cue":                []byte("module: \"s3\"\nversion: \"1.0.0\""),
		"v1/_version.cue":            []byte("apiVersion: \"v1\""),
		"v1/definitions/bucket.yaml": []byte("apiVersion: core.oam.dev/v1beta1\nkind: ComponentDefinition\nmetadata:\n  name: atmos-s3-v1\n"),
	}
	mod, err := module.ParseModule(m)
	require.NoError(t, err)
	require.Equal(t, "s3", mod.Name)
	require.Contains(t, mod.Lines, "v1")
}
