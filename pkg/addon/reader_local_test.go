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

package addon

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalReader(t *testing.T) {
	r := localReader{name: "local", dir: "./testdata/local"}
	m, err := r.ListAddonMeta()
	assert.NoError(t, err)
	assert.Equal(t, len(m["local"].Items), 2)

	file, err := r.ReadFile("metadata.yaml")
	assert.NoError(t, err)
	assert.Equal(t, file, metaFile)

	file, err = r.ReadFile("resources/parameter.cue")
	assert.NoError(t, err)
	assert.Equal(t, true, strings.Contains(file, parameterFile))
}

const (
	metaFile = `name: test-local-addon
version: 1.0.0
description: This is a addon for test when install addon from local
icon: https://www.terraform.io/assets/images/logo-text-8c3ba8a6.svg
url: https://terraform.io/

tags: []

deployTo:
  control_plane: true
  runtime_cluster: false

dependencies: []

invisible: false`

	parameterFile = `parameter: {
	// test wrong parameter
	example: *"default"
}`
)
