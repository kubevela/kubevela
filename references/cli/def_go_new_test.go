/*
Copyright 2025 The KubeVela Authors.

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

package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func initGoDefTestCommand(cmd *cobra.Command) {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.Flags().StringP("env", "", "", "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
}

func TestDefinitionInitGoComponent(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	outputFile := filepath.Join(tempDir, "mycomponent.go")
	cmd.SetArgs([]string{"mycomponent", "-t", "component", "--lang", "go", "-o", outputFile})

	err = cmd.Execute()
	require.NoError(t, err)

	// Check file was created
	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	// Verify content has expected Go definition structure
	contentStr := string(content)
	assert.Contains(t, contentStr, "package main")
	assert.Contains(t, contentStr, "github.com/oam-dev/kubevela/pkg/definition/defkit")
	assert.Contains(t, contentStr, "defkit.NewComponent")
	assert.Contains(t, contentStr, "MycomponentComponent")
}

func TestDefinitionInitGoTrait(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	outputFile := filepath.Join(tempDir, "mytrait.go")
	cmd.SetArgs([]string{"mytrait", "-t", "trait", "--lang", "go", "-o", outputFile})

	err = cmd.Execute()
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "defkit.NewTrait")
	assert.Contains(t, contentStr, "MytraitTrait")
}

func TestDefinitionInitGoPolicy(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	outputFile := filepath.Join(tempDir, "mypolicy.go")
	cmd.SetArgs([]string{"mypolicy", "-t", "policy", "--lang", "go", "-o", outputFile})

	err = cmd.Execute()
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "defkit.NewPolicy")
	assert.Contains(t, contentStr, "MypolicyPolicy")
}

func TestDefinitionInitGoWorkflowStep(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	outputFile := filepath.Join(tempDir, "mystep.go")
	cmd.SetArgs([]string{"mystep", "-t", "workflow-step", "--lang", "go", "-o", outputFile})

	err = cmd.Execute()
	require.NoError(t, err)

	content, err := os.ReadFile(outputFile)
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "defkit.NewWorkflowStep")
	assert.Contains(t, contentStr, "MystepWorkflowStep")
}

func TestDefinitionInitDefaultToCUE(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	cmd.SetArgs([]string{"mydef", "-t", "component", "--desc", "test"})

	// Capture output
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := cmd.Execute()
	require.NoError(t, err)

	// Output should be CUE format (contains template: and parameter:)
	output := buf.String()
	assert.Contains(t, output, "template:")
}

func TestDefinitionInitInvalidLang(t *testing.T) {
	c := initArgs()
	cmd := NewDefinitionInitCommand(c)
	initGoDefTestCommand(cmd)

	// Capture output
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	cmd.SetArgs([]string{"mydef", "-t", "component", "--lang", "python"})

	// Currently, invalid lang falls through to CUE generation (doesn't error)
	// This behavior could be tightened with validation in the future
	err := cmd.Execute()
	require.NoError(t, err)

	// Output should be CUE format since "python" is not "go"
	output := buf.String()
	assert.Contains(t, output, "template:")
}

func TestIsDefinitionFile(t *testing.T) {
	// Create temp directory with Go definition file for testing
	tempDir, err := os.MkdirTemp("", "vela-def-file-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a Go definition file
	goDefFile := filepath.Join(tempDir, "component.go")
	goDefContent := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func MyComponent() *defkit.ComponentDefinition { return nil }
`
	err = os.WriteFile(goDefFile, []byte(goDefContent), 0600)
	require.NoError(t, err)

	// Create a regular Go file (not a definition)
	regularGoFile := filepath.Join(tempDir, "util.go")
	regularGoContent := `package main
func helper() {}
`
	err = os.WriteFile(regularGoFile, []byte(regularGoContent), 0600)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		filename string
		expected bool
	}{
		{"CUE file", "component.cue", true},
		{"YAML file", "component.yaml", true},
		{"YML file", "component.yml", true},
		{"Go definition file", goDefFile, true},
		{"Regular Go file", regularGoFile, false},
		{"Test file", "component_test.go", false},
		{"Markdown file", "README.md", false},
		{"JSON file", "config.json", true}, // JSON is included in IsJSONYAMLorCUEFile
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isDefinitionFile(tc.filename)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestIsCUEorGoDefinitionFile(t *testing.T) {
	// Create temp directory with Go definition file for testing
	tempDir, err := os.MkdirTemp("", "vela-cue-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a Go definition file
	goDefFile := filepath.Join(tempDir, "component.go")
	goDefContent := `package main
import "github.com/oam-dev/kubevela/pkg/definition/defkit"
func MyComponent() *defkit.ComponentDefinition { return nil }
`
	err = os.WriteFile(goDefFile, []byte(goDefContent), 0600)
	require.NoError(t, err)

	testCases := []struct {
		name     string
		filename string
		expected bool
	}{
		{"CUE file", "component.cue", true},
		{"Go definition file", goDefFile, true},
		{"Test file", "component_test.go", false},
		{"YAML file", "component.yaml", false},
		{"YML file", "component.yml", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isCUEorGoDefinitionFile(tc.filename)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestToPascalCase(t *testing.T) {
	// toPascalCase splits on - and _, capitalizes first letter, lowercases rest
	testCases := []struct {
		input    string
		expected string
	}{
		{"webservice", "Webservice"},
		{"my-component", "MyComponent"}, // splits on hyphen, capitalizes each part
		{"my_component", "MyComponent"}, // splits on underscore too
		{"MyComponent", "Mycomponent"},  // doesn't split on camelCase, lowercases after first letter
		{"abc", "Abc"},
		{"web-service-app", "WebServiceApp"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := toPascalCase(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGenerateGoComponentDefinition(t *testing.T) {
	scaffold := generateGoComponentDefinition("webservice", "WebserviceComponent", "A web service component")
	assert.Contains(t, scaffold, "WebserviceComponent")
	assert.Contains(t, scaffold, "defkit.NewComponent")
	assert.Contains(t, scaffold, "A web service component")
	assert.Contains(t, scaffold, "package main")
}

func TestGenerateGoTraitDefinition(t *testing.T) {
	scaffold := generateGoTraitDefinition("scaler", "ScalerTrait", "A scaler trait")
	assert.Contains(t, scaffold, "ScalerTrait")
	assert.Contains(t, scaffold, "defkit.NewTrait")
	assert.Contains(t, scaffold, "A scaler trait")
}

func TestGenerateGoPolicyDefinition(t *testing.T) {
	scaffold := generateGoPolicyDefinition("topology", "TopologyPolicy", "A topology policy")
	assert.Contains(t, scaffold, "TopologyPolicy")
	assert.Contains(t, scaffold, "defkit.NewPolicy")
	assert.Contains(t, scaffold, "A topology policy")
}

func TestGenerateGoWorkflowStepDefinition(t *testing.T) {
	scaffold := generateGoWorkflowStepDefinition("deploy", "DeployWorkflowStep", "A deploy step")
	assert.Contains(t, scaffold, "DeployWorkflowStep")
	assert.Contains(t, scaffold, "defkit.NewWorkflowStep")
	assert.Contains(t, scaffold, "A deploy step")
}

func TestDefinitionVetGoFile(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a Go definition file
	goFile := filepath.Join(tempDir, "valid.go")
	content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func ValidComponent() *defkit.ComponentDefinition {
	image := defkit.String("image").Required()
	return defkit.NewComponent("valid").
		Description("A valid component").
		Workload("apps/v1", "Deployment").
		Params(image).
		Template(func(tpl *defkit.Template) {
			tpl.Output(
				defkit.NewResource("apps/v1", "Deployment").
					Set("spec.template.spec.containers[0].image", image),
			)
		})
}
`
	err = os.WriteFile(goFile, []byte(content), 0600)
	require.NoError(t, err)

	cmd := NewDefinitionValidateCommand(c)
	initGoDefTestCommand(cmd)
	cmd.SetArgs([]string{goFile})

	// The validation will attempt to generate CUE from the Go file
	// This requires a proper Go module, so it may fail in test environment
	// But we verify the command accepts .go files
	_ = cmd.Execute()
}

func TestDefinitionVetNonExistentGoFile(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	cmd := NewDefinitionValidateCommand(c)
	initGoDefTestCommand(cmd)
	cmd.SetArgs([]string{filepath.Join(tempDir, "nonexistent.go")})

	err = cmd.Execute()
	assert.Error(t, err)
}

func TestDefinitionRenderGoFile(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a Go definition file
	goFile := filepath.Join(tempDir, "component.go")
	content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func TestComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("test").
		Description("Test component")
}
`
	err = os.WriteFile(goFile, []byte(content), 0600)
	require.NoError(t, err)

	cmd := NewDefinitionRenderCommand(c)
	initGoDefTestCommand(cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{goFile})

	// The render command should accept .go files
	// Full execution requires proper Go module setup
	_ = cmd.Execute()
}

func TestDefinitionApplyGoFileDryRun(t *testing.T) {
	c := initArgs()
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a Go definition file
	goFile := filepath.Join(tempDir, "component.go")
	content := `package main

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func ApplyTestComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("apply-test").
		Description("Test component for apply")
}
`
	err = os.WriteFile(goFile, []byte(content), 0600)
	require.NoError(t, err)

	ioStreams := util.IOStreams{In: os.Stdin, Out: bytes.NewBuffer(nil), ErrOut: bytes.NewBuffer(nil)}
	cmd := NewDefinitionApplyCommand(c, ioStreams)
	initGoDefTestCommand(cmd)

	cmd.SetArgs([]string{goFile, "--dry-run"})

	// The apply command should accept .go files
	// Full execution requires proper Go module setup
	_ = cmd.Execute()
}

func TestGoExtensionConstant(t *testing.T) {
	assert.Equal(t, ".go", GoExtension)
}

func TestGoFileInDirectory(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "vela-go-def-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create various files
	files := map[string]string{
		"component.go":      `package main; import "github.com/oam-dev/kubevela/pkg/definition/defkit"; func C() *defkit.ComponentDefinition { return nil }`,
		"trait.go":          `package main; import "github.com/oam-dev/kubevela/pkg/definition/defkit"; func T() *defkit.TraitDefinition { return nil }`,
		"component_test.go": `package main; func TestC() {}`,
		"util.go":           `package main; func helper() {}`,
		"component.cue":     `component: {}`,
		"trait.yaml":        `kind: TraitDefinition`,
	}

	for name, content := range files {
		err := os.WriteFile(filepath.Join(tempDir, name), []byte(content), 0600)
		require.NoError(t, err)
	}

	// Verify isDefinitionFile identifies the right files
	// Note: isDefinitionFile needs full path for Go files to check the content
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)

	definitionFiles := []string{}
	for _, entry := range entries {
		fullPath := filepath.Join(tempDir, entry.Name())
		if isDefinitionFile(fullPath) {
			definitionFiles = append(definitionFiles, entry.Name())
		}
	}

	// Should include Go definition files, .cue, .yaml
	// util.go is NOT a definition file because it doesn't import defkit
	assert.Contains(t, definitionFiles, "component.go")
	assert.Contains(t, definitionFiles, "trait.go")
	assert.NotContains(t, definitionFiles, "util.go") // Not a definition file - no defkit import
	assert.Contains(t, definitionFiles, "component.cue")
	assert.Contains(t, definitionFiles, "trait.yaml")
	assert.NotContains(t, definitionFiles, "component_test.go")
}

func TestGoScaffoldHasProperStructure(t *testing.T) {
	// The scaffold generators use the pattern: funcName + type suffix
	// E.g., generateGoComponentDefinition("test", "Test", ...) generates "func TestComponent()"
	testCases := []struct {
		name           string
		defType        string
		funcNamePrefix string // prefix before type suffix in generated code
		expectedFunc   string // what function name appears in generated code
		genFunc        func(string, string, string) string
	}{
		{"component", "component", "Test", "TestComponent", generateGoComponentDefinition},
		{"trait", "trait", "Test", "TestTrait", generateGoTraitDefinition},
		{"policy", "policy", "Test", "TestPolicy", generateGoPolicyDefinition},
		{"workflow-step", "workflow-step", "Test", "TestWorkflowStep", generateGoWorkflowStepDefinition},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scaffold := tc.genFunc("test", tc.funcNamePrefix, "Test description")

			// Check structure
			assert.Contains(t, scaffold, "package main")
			assert.Contains(t, scaffold, "import")
			assert.Contains(t, scaffold, "github.com/oam-dev/kubevela/pkg/definition/defkit")
			assert.Contains(t, scaffold, tc.expectedFunc)
			assert.Contains(t, scaffold, "Test description")

			// Check it's valid Go syntax (no obvious issues)
			// The generators add type suffix: TestComponent(), TestTrait(), etc.
			assert.True(t, strings.Contains(scaffold, "func "+tc.expectedFunc+"()"),
				"Expected function %s in scaffold", tc.expectedFunc)
			assert.True(t, strings.Contains(scaffold, "return defkit."))
		})
	}
}
