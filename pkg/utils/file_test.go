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

	"gotest.tools/assert"
)

func TestIsEmptyDir(t *testing.T) {
	// Test with an empty dir
	err := os.Mkdir("testdir", 0750)
	assert.NilError(t, err)
	defer func() {
		_ = os.RemoveAll("testdir")
	}()
	isEmptyDir, err := IsEmptyDir("testdir")
	assert.Equal(t, isEmptyDir, true)
	assert.NilError(t, err)
	// Test with a file
	err = os.WriteFile(filepath.Join("testdir", "testfile"), []byte("test"), 0644)
	assert.NilError(t, err)
	isEmptyDir, err = IsEmptyDir(filepath.Join("testdir", "testfile"))
	assert.Equal(t, isEmptyDir, false)
	assert.Equal(t, err != nil, true)
	// Test with a non-empty dir
	isEmptyDir, err = IsEmptyDir("testdir")
	assert.Equal(t, isEmptyDir, false)
	assert.NilError(t, err)
}
