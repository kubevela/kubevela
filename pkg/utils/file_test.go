/*
Copyright 2022 The KubeVela Authors.

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

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsEmptyDir(t *testing.T) {
	// Test with an empty dir
	err := os.Mkdir("testdir", 0750)
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll("testdir")
	}()
	isEmptyDir, err := IsEmptyDir("testdir")
	assert.Equal(t, isEmptyDir, true)
	assert.NoError(t, err)
	// Test with a file
	err = os.WriteFile(filepath.Join("testdir", "testfile"), []byte("test"), 0644)
	assert.NoError(t, err)
	isEmptyDir, err = IsEmptyDir(filepath.Join("testdir", "testfile"))
	assert.Equal(t, isEmptyDir, false)
	assert.Equal(t, err != nil, true)
	// Test with a non-empty dir
	isEmptyDir, err = IsEmptyDir("testdir")
	assert.Equal(t, isEmptyDir, false)
	assert.NoError(t, err)
}

func TestGetFilenameFromLocalOrRemote(t *testing.T) {
	cases := []struct {
		path     string
		filename string
	}{
		{
			path:     "./name",
			filename: "name",
		},
		{
			path:     "name",
			filename: "name",
		},
		{
			path:     "../name.js",
			filename: "name",
		},
		{
			path:     "../name.tar.zst",
			filename: "name.tar",
		},
		{
			path:     "https://some.com/file.js",
			filename: "file",
		},
		{
			path:     "https://some.com/file",
			filename: "file",
		},
	}

	for _, c := range cases {
		n, _ := GetFilenameFromLocalOrRemote(c.path)
		assert.Equal(t, c.filename, n)
	}
}

func TestIsJSONYAMLorCUEFile(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test is json file",
			args: args{
				path: "test.json",
			},
			want: true,
		},
		{
			name: "test is yaml file",
			args: args{
				path: "test.yaml",
			},
			want: true,
		},
		{
			name: "test is cue file",
			args: args{
				path: "test.cue",
			},
			want: true,
		},
		{
			name: "test is not json/yaml/cue file",
			args: args{
				path: "test.txt",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsJSONYAMLorCUEFile(tt.args.path), "IsJSONYAMLorCUEFile(%v)", tt.args.path)
		})
	}
}
