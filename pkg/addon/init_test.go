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

	"github.com/stretchr/testify/assert"
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
		assert.NoError(t, err)
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
	assert.NoError(t, err)
	defer os.RemoveAll("with-helm")
	_, err = os.Stat(filepath.Join("with-helm", ResourcesDirName, "helm.cue"))
	assert.NoError(t, err)

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
	assert.NoError(t, err)
	defer os.RemoveAll("with-refobj")
	_, err = os.Stat(filepath.Join("with-refobj", ResourcesDirName, "from-url.cue"))
	assert.NoError(t, err)
}

func TestInitCmd_CreateScaffoldWithGoDef(t *testing.T) {
	// Test godef scaffolding creation (basic, no specific definitions)
	cmd := InitCmd{
		AddonName:   "godef-addon",
		Path:        "godef-addon",
		EnableGoDef: true,
		NoSamples:   true, // Skip CUE samples to focus on godef
	}
	err := cmd.CreateScaffold()
	assert.NoError(t, err)
	defer os.RemoveAll("godef-addon")

	// Verify godef directory structure
	godefPath := filepath.Join("godef-addon", GoDefDirName)
	_, err = os.Stat(godefPath)
	assert.NoError(t, err, "godef directory should exist")

	// Verify module.yaml
	moduleYAMLPath := filepath.Join(godefPath, GoDefModuleFileName)
	data, err := os.ReadFile(moduleYAMLPath)
	assert.NoError(t, err, "module.yaml should exist")
	assert.Contains(t, string(data), "godef-addon", "module.yaml should contain addon name")
	assert.Contains(t, string(data), "defkit.oam.dev/v1", "module.yaml should have correct apiVersion")

	// Verify go.mod
	goModPath := filepath.Join(godefPath, "go.mod")
	data, err = os.ReadFile(goModPath)
	assert.NoError(t, err, "go.mod should exist")
	assert.Contains(t, string(data), "godef-addon", "go.mod should contain addon name")
	assert.Contains(t, string(data), "go 1.23", "go.mod should specify Go version")

	// Verify all four directories exist
	for _, dir := range []string{"components", "traits", "policies", "workflowsteps"} {
		dirPath := filepath.Join(godefPath, dir)
		_, err = os.Stat(dirPath)
		assert.NoError(t, err, "%s directory should exist", dir)

		// Verify doc.go exists in each directory
		docPath := filepath.Join(dirPath, "doc.go")
		data, err = os.ReadFile(docPath)
		assert.NoError(t, err, "doc.go should exist in %s", dir)
		assert.Contains(t, string(data), "package "+dir)
	}
}

func TestInitCmd_CreateScaffoldWithGoDefAndDefinitions(t *testing.T) {
	// Test godef scaffolding with specific definitions
	cmd := InitCmd{
		AddonName:       "godef-defs",
		Path:            "godef-defs",
		EnableGoDef:     true,
		GoDefComponents: "webservice,worker",
		GoDefTraits:     "scaler",
		NoSamples:       true,
	}
	err := cmd.CreateScaffold()
	assert.NoError(t, err)
	defer os.RemoveAll("godef-defs")

	godefPath := filepath.Join("godef-defs", GoDefDirName)

	// Verify component scaffolds were created
	webservicePath := filepath.Join(godefPath, "components", "webservice.go")
	data, err := os.ReadFile(webservicePath)
	assert.NoError(t, err, "webservice.go should exist")
	assert.Contains(t, string(data), "package components")
	assert.Contains(t, string(data), "WebserviceComponent")
	assert.Contains(t, string(data), `defkit.NewComponent("webservice")`)

	workerPath := filepath.Join(godefPath, "components", "worker.go")
	data, err = os.ReadFile(workerPath)
	assert.NoError(t, err, "worker.go should exist")
	assert.Contains(t, string(data), "WorkerComponent")

	// Verify trait scaffold was created
	scalerPath := filepath.Join(godefPath, "traits", "scaler.go")
	data, err = os.ReadFile(scalerPath)
	assert.NoError(t, err, "scaler.go should exist")
	assert.Contains(t, string(data), "package traits")
	assert.Contains(t, string(data), "ScalerTrait")
	assert.Contains(t, string(data), `defkit.NewTrait("scaler")`)
}

func TestInitCmd_CreateScaffoldWithGoDefAndSamples(t *testing.T) {
	// Test godef scaffolding with CUE samples enabled
	cmd := InitCmd{
		AddonName:   "mixed-addon",
		Path:        "mixed-addon",
		EnableGoDef: true,
		NoSamples:   false, // Include CUE samples
	}
	err := cmd.CreateScaffold()
	assert.NoError(t, err)
	defer os.RemoveAll("mixed-addon")

	// Verify both godef and CUE samples exist
	godefPath := filepath.Join("mixed-addon", GoDefDirName)
	_, err = os.Stat(godefPath)
	assert.NoError(t, err, "godef directory should exist")

	// Verify CUE samples also exist
	cueSamplePath := filepath.Join("mixed-addon", DefinitionsDirName, "mytrait.cue")
	_, err = os.Stat(cueSamplePath)
	assert.NoError(t, err, "CUE sample should also exist")
}

func TestInitCmd_GoDefScaffoldContents(t *testing.T) {
	cmd := InitCmd{
		AddonName:   "content-test",
		EnableGoDef: true,
	}

	// Call createGoDefScaffold directly
	cmd.createGoDefScaffold()

	// Verify correct number of files (module.yaml, go.mod, 4x doc.go)
	assert.Len(t, cmd.GoDefFiles, 6, "should have 6 godef files")

	// Verify file names
	fileNames := make(map[string]bool)
	for _, f := range cmd.GoDefFiles {
		fileNames[f.Name] = true
	}

	expectedFiles := []string{
		filepath.Join(GoDefDirName, GoDefModuleFileName),
		filepath.Join(GoDefDirName, "go.mod"),
		filepath.Join(GoDefDirName, "components", "doc.go"),
		filepath.Join(GoDefDirName, "traits", "doc.go"),
		filepath.Join(GoDefDirName, "policies", "doc.go"),
		filepath.Join(GoDefDirName, "workflowsteps", "doc.go"),
	}

	for _, expected := range expectedFiles {
		assert.True(t, fileNames[expected], "should contain file: %s", expected)
	}
}

func TestInitCmd_GoDefScaffoldWithDefinitions(t *testing.T) {
	cmd := InitCmd{
		AddonName:       "scaffold-test",
		EnableGoDef:     true,
		GoDefComponents: "api,backend",
		GoDefTraits:     "ingress",
		GoDefPolicies:   "topology",
	}

	cmd.createGoDefScaffold()

	// Should have: module.yaml, go.mod, 4x doc.go, 2x component, 1x trait, 1x policy
	assert.Len(t, cmd.GoDefFiles, 10, "should have 10 godef files")

	fileNames := make(map[string]bool)
	for _, f := range cmd.GoDefFiles {
		fileNames[f.Name] = true
	}

	// Verify scaffold files were created
	assert.True(t, fileNames[filepath.Join(GoDefDirName, "components", "api.go")])
	assert.True(t, fileNames[filepath.Join(GoDefDirName, "components", "backend.go")])
	assert.True(t, fileNames[filepath.Join(GoDefDirName, "traits", "ingress.go")])
	assert.True(t, fileNames[filepath.Join(GoDefDirName, "policies", "topology.go")])
}

func TestGoDefTemplates(t *testing.T) {
	// Verify template content quality
	t.Run("module.yaml template", func(t *testing.T) {
		assert.Contains(t, godefModuleYAMLTemplate, "apiVersion: defkit.oam.dev/v1")
		assert.Contains(t, godefModuleYAMLTemplate, "kind: DefinitionModule")
		assert.Contains(t, godefModuleYAMLTemplate, "ADDON_NAME")
	})

	t.Run("go.mod template", func(t *testing.T) {
		assert.Contains(t, godefGoModTemplate, "module github.com/my-org/ADDON_NAME/godef")
		assert.Contains(t, godefGoModTemplate, "go 1.23")
		assert.Contains(t, godefGoModTemplate, "require github.com/oam-dev/kubevela")
	})

	t.Run("components doc.go template", func(t *testing.T) {
		assert.Contains(t, godefComponentsDocTemplate, "package components")
		assert.Contains(t, godefComponentsDocTemplate, "ComponentDefinition")
		assert.Contains(t, godefComponentsDocTemplate, "defkit.NewComponent")
	})

	t.Run("traits doc.go template", func(t *testing.T) {
		assert.Contains(t, godefTraitsDocTemplate, "package traits")
		assert.Contains(t, godefTraitsDocTemplate, "TraitDefinition")
		assert.Contains(t, godefTraitsDocTemplate, "defkit.NewTrait")
	})

	t.Run("policies doc.go template", func(t *testing.T) {
		assert.Contains(t, godefPoliciesDocTemplate, "package policies")
		assert.Contains(t, godefPoliciesDocTemplate, "PolicyDefinition")
		assert.Contains(t, godefPoliciesDocTemplate, "defkit.NewPolicy")
	})

	t.Run("workflowsteps doc.go template", func(t *testing.T) {
		assert.Contains(t, godefWorkflowStepsDocTemplate, "package workflowsteps")
		assert.Contains(t, godefWorkflowStepsDocTemplate, "WorkflowStepDefinition")
		assert.Contains(t, godefWorkflowStepsDocTemplate, "defkit.NewWorkflowStep")
	})
}

func TestGenerateScaffolds(t *testing.T) {
	t.Run("component scaffold", func(t *testing.T) {
		scaffold := generateComponentScaffold("my-service")
		assert.Contains(t, scaffold, "package components")
		assert.Contains(t, scaffold, "MyServiceComponent")
		assert.Contains(t, scaffold, `defkit.NewComponent("my-service")`)
		assert.Contains(t, scaffold, "defkit.Register")
	})

	t.Run("trait scaffold", func(t *testing.T) {
		scaffold := generateTraitScaffold("auto-scale")
		assert.Contains(t, scaffold, "package traits")
		assert.Contains(t, scaffold, "AutoScaleTrait")
		assert.Contains(t, scaffold, `defkit.NewTrait("auto-scale")`)
	})

	t.Run("policy scaffold", func(t *testing.T) {
		scaffold := generatePolicyScaffold("topology")
		assert.Contains(t, scaffold, "package policies")
		assert.Contains(t, scaffold, "TopologyPolicy")
		assert.Contains(t, scaffold, `defkit.NewPolicy("topology")`)
	})

	t.Run("workflow step scaffold", func(t *testing.T) {
		scaffold := generateWorkflowStepScaffold("deploy")
		assert.Contains(t, scaffold, "package workflowsteps")
		assert.Contains(t, scaffold, "DeployStep")
		assert.Contains(t, scaffold, `defkit.NewWorkflowStep("deploy")`)
	})
}

func TestToCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"webservice", "Webservice"},
		{"my-service", "MyService"},
		{"auto-scale-v2", "AutoScaleV2"},
		{"simple", "Simple"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, toCamelCase(tt.input))
		})
	}
}

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"one", []string{"one"}},
		{"one,two", []string{"one", "two"}},
		{"one, two, three", []string{"one", "two", "three"}},
		{" spaced , values ", []string{"spaced", "values"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseCommaSeparated(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
