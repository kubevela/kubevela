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

package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateIfNotExist(t *testing.T) {
	testDir := "TestCreateIfNotExist"
	defer os.RemoveAll(testDir)

	normalCreate := filepath.Join(testDir, "normalCase")
	_, err := CreateIfNotExist(normalCreate)
	assert.NoError(t, err)
	fi, err := os.Stat(normalCreate)
	assert.NoError(t, err)
	assert.Equal(t, true, fi.IsDir())

	normalNestCreate := filepath.Join(testDir, "nested", "normalCase")
	_, err = CreateIfNotExist(normalNestCreate)
	assert.NoError(t, err)
	fi, err = os.Stat(normalNestCreate)
	assert.NoError(t, err)
	assert.Equal(t, true, fi.IsDir())
}

func TestGetVelaHomeDir(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		preFunc func()
		postFun func()
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "test get vela home dir from env",
			preFunc: func() {
				_ = os.Setenv(VelaHomeEnv, "/tmp")
			},
			want:    "/tmp",
			wantErr: assert.NoError,
		},
		{
			name: "test use default vela home dir",
			preFunc: func() {
				_ = os.Unsetenv(VelaHomeEnv)
			},
			want:    filepath.Join(os.Getenv("HOME"), defaultVelaHome),
			wantErr: assert.NoError,
			postFun: func() {
				_ = os.RemoveAll(filepath.Join(os.Getenv("HOME"), defaultVelaHome))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.preFunc != nil {
				tt.preFunc()
			}
			defer func() {
				_ = os.Unsetenv(VelaHomeEnv)
			}()
			got, err := GetVelaHomeDir()
			if !tt.wantErr(t, err, "GetVelaHomeDir()") {
				return
			}

			assert.Equalf(t, tt.want, got, "GetVelaHomeDir()")
		})
	}
}
