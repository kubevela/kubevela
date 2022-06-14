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
	"path"
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

func TestWriteHelmComponentTemplate(t *testing.T) {
	resourceTmpl := HelmComponentTemplate{}
	resourceTmpl.Output.Type = "helm"
	resourceTmpl.Output.Properties.RepoType = "helm"
	resourceTmpl.Output.Properties.URL = "https://charts.bitnami.com/bitnami"
	resourceTmpl.Output.Properties.Chart = "bitnami/nginx"
	resourceTmpl.Output.Properties.Version = "12.0.4"
	err := writeHelmComponentTemplate(resourceTmpl, "test.cue")
	assert.NilError(t, err)
	defer func() {
		_ = os.Remove("test.cue")
	}()
	data, err := os.ReadFile("test.cue")
	assert.NilError(t, err)
	expected := `output: {
	type: "helm"
	properties: {
		url:      "https://charts.bitnami.com/bitnami"
		repoType: "helm"
		chart:    "bitnami/nginx"
		version:  "12.0.4"
	}
}`
	assert.Equal(t, string(data), expected)
}

func TestCreateAddonFromHelmChart(t *testing.T) {
	err := CreateAddonFromHelmChart("", "", "", "bitnami/nginx", "12.0.4")
	assert.ErrorContains(t, err, "should not be empty")

	checkFiles := func(base string) {
		fileList := []string{
			"definitions",
			path.Join("resources", base+".cue"),
			"schemas",
			MetadataFileName,
			ReadmeFileName,
			TemplateFileName,
		}
		for _, file := range fileList {
			_, err = os.Stat(path.Join(base, file))
			assert.NilError(t, err)
		}
	}

	// Empty dir already exists
	_ = os.MkdirAll("test-addon", 0755)
	err = CreateAddonFromHelmChart("test-addon", "./test-addon", "https://charts.bitnami.com/bitnami", "bitnami/nginx", "12.0.4")
	checkFiles("test-addon")
	defer func() {
		_ = os.RemoveAll("test-addon")
	}()

	// Non-empty dir already exists
	err = CreateAddonFromHelmChart("test-addon", "", "https://charts.bitnami.com/bitnami", "bitnami/nginx", "12.0.4")
	assert.ErrorContains(t, err, "not empty")

	// Name already taken
	err = os.WriteFile("already-taken", []byte{}, 0644)
	assert.NilError(t, err)
	defer func() {
		_ = os.Remove("already-taken")
	}()
	err = CreateAddonFromHelmChart("already-taken", "", "https://charts.bitnami.com/bitnami", "bitnami/nginx", "12.0.4")
	assert.ErrorContains(t, err, "can't create")

	// Invalid addon name
	err = CreateAddonFromHelmChart("/", "", "https://charts.bitnami.com/bitnami", "bitnami/nginx", "12.0.4")
	assert.ErrorContains(t, err, "should only")

	// Invalid URL
	err = CreateAddonFromHelmChart("invalid-url", "", "invalid-url", "bitnami/nginx", "12.0.4")
	assert.ErrorContains(t, err, "invalid helm repo url")
}
