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

package addon

import (
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
)

func TestCheckAddonName(t *testing.T) {
	var err error

	err = CheckAddonName("")
	assert.ErrorContains(t, err, "should not be empty")

	invalidNames := []string{
		"-addon",
		"addon-",
		"Caps",
		"=",
		".",
	}
	for _, name := range invalidNames {
		err = CheckAddonName(name)
		assert.ErrorContains(t, err, "should only")
	}

	validNames := []string{
		"addon-name",
		"3-addon-name",
		"addon-name-3",
		"addon",
	}
	for _, name := range validNames {
		err = CheckAddonName(name)
		assert.NilError(t, err)
	}
}

func TestInitCmd_CreateScaffold(t *testing.T) {
	var err error

	// empty addon name or path
	cmd := InitCmd{}
	err = cmd.CreateScaffold()
	assert.ErrorContains(t, err, "be empty")

	// invalid addon name
	cmd = InitCmd{
		AddonName: "-name",
		Path:      "name",
	}
	err = cmd.CreateScaffold()
	assert.ErrorContains(t, err, "should only")

	// dir already exists
	cmd = InitCmd{
		AddonName: "name",
		Path:      "testdata",
	}
	err = cmd.CreateScaffold()
	assert.ErrorContains(t, err, "cannot create")

	// with helm component
	cmd = InitCmd{
		AddonName:        "with-helm",
		Path:             "with-helm",
		HelmRepoURL:      "https://charts.bitnami.com/bitnami",
		HelmChartVersion: "12.0.0",
		HelmChartName:    "nginx",
	}
	err = cmd.CreateScaffold()
	assert.NilError(t, err)
	defer os.RemoveAll("with-helm")
	_, err = os.Stat(filepath.Join("with-helm", ResourcesDirName, "helm.cue"))
	assert.NilError(t, err)

	// with ref-obj
	cmd = InitCmd{
		AddonName:  "with-refobj",
		Path:       "with-refobj",
		RefObjURLs: []string{"https:"},
	}
	err = cmd.CreateScaffold()
	assert.ErrorContains(t, err, "not a valid url")
	cmd.RefObjURLs[0] = "https://some.com"
	err = cmd.CreateScaffold()
	assert.NilError(t, err)
	defer os.RemoveAll("with-refobj")
	_, err = os.Stat(filepath.Join("with-refobj", ResourcesDirName, "from-url.cue"))
	assert.NilError(t, err)
}
