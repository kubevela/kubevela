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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateModuleName(t *testing.T) {
	cases := []struct {
		name      string
		wantError bool
	}{
		{name: "s3", wantError: false},
		{name: "aws-s3", wantError: false},
		{name: "", wantError: true},
	}
	for _, c := range cases {
		err := validateModuleName(c.name, "modules/x/_module.cue")
		if c.wantError {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "_module.cue")
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestValidateModuleVersion(t *testing.T) {
	cases := []struct {
		version   string
		wantError bool
	}{
		{version: "1.0.0", wantError: false},
		{version: "0.1.0", wantError: false},
		{version: "1.2.3-beta.1", wantError: false},
		{version: "1.2.3+build.5", wantError: false},
		{version: "", wantError: true},
		{version: "latest", wantError: true},
		{version: "v1", wantError: true},
		{version: "1.x", wantError: true},
		{version: "1.2", wantError: true},
		{version: "v1.2.3", wantError: true},
	}
	for _, c := range cases {
		err := validateModuleVersion(c.version, "modules/x/_module.cue")
		if c.wantError {
			assert.Error(t, err, "version %q should be rejected", c.version)
			assert.Contains(t, err.Error(), "_module.cue")
		} else {
			assert.NoError(t, err, "version %q should be accepted", c.version)
		}
	}
}

func TestValidateAPIVersion(t *testing.T) {
	cases := []struct {
		apiVersion string
		wantError  bool
	}{
		{apiVersion: "v1", wantError: false},
		{apiVersion: "v2", wantError: false},
		{apiVersion: "v1beta1", wantError: false},
		{apiVersion: "v1alpha2", wantError: false},
		{apiVersion: "1.0", wantError: true},
		{apiVersion: "latest", wantError: true},
		{apiVersion: "", wantError: true},
		{apiVersion: "v1.2", wantError: true},
	}
	for _, c := range cases {
		err := validateAPIVersion(c.apiVersion, "modules/x/v1/_version.cue")
		if c.wantError {
			assert.Error(t, err, "apiVersion %q should be rejected", c.apiVersion)
			assert.Contains(t, err.Error(), "_version.cue")
		} else {
			assert.NoError(t, err, "apiVersion %q should be accepted", c.apiVersion)
		}
	}
}

func TestValidateDefinitionName(t *testing.T) {
	cases := []struct {
		name string
		def  map[string]interface{}
		want bool // wantError
	}{
		{
			name: "named definition",
			def:  map[string]interface{}{"metadata": map[string]interface{}{"name": "bucket"}},
			want: false,
		},
		{
			name: "empty name",
			def:  map[string]interface{}{"metadata": map[string]interface{}{"name": ""}},
			want: true,
		},
		{
			name: "missing metadata",
			def:  map[string]interface{}{},
			want: true,
		},
	}
	for _, c := range cases {
		err := validateDefinitionName(c.def, "modules/x/v1/definitions/widget.yaml")
		if c.want {
			assert.Error(t, err, c.name)
		} else {
			assert.NoError(t, err, c.name)
		}
	}
}

func TestValidateLines(t *testing.T) {
	assert.Error(t, validateLines(map[string]Line{}))
	assert.NoError(t, validateLines(map[string]Line{"v1": {APIVersion: "v1"}}))
}
