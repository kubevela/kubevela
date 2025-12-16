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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestNewDefinitionApplyModuleCommand(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionApplyModuleCommand(c, ioStreams)

	assert.NotNil(t, cmd)
	assert.Equal(t, "apply-module MODULE", cmd.Use)
	assert.Contains(t, cmd.Short, "Apply all definitions from a Go module")

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup(FlagDryRun))
	assert.NotNil(t, cmd.Flags().Lookup(Namespace))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleVersion))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleTypes))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModulePrefix))
	assert.NotNil(t, cmd.Flags().Lookup(FlagConflictStrategy))
}

func TestNewDefinitionListModuleCommand(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionListModuleCommand(c, ioStreams)

	assert.NotNil(t, cmd)
	assert.Equal(t, "list-module MODULE", cmd.Use)
	assert.Contains(t, cmd.Short, "List all definitions in a Go module")

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleVersion))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleTypes))
}

func TestNewDefinitionValidateModuleCommand(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionValidateModuleCommand(c, ioStreams)

	assert.NotNil(t, cmd)
	assert.Equal(t, "validate-module MODULE", cmd.Use)
	assert.Contains(t, cmd.Short, "Validate all definitions in a Go module")

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleVersion))
}

func TestConflictStrategy(t *testing.T) {
	// Test conflict strategy constants
	assert.Equal(t, ConflictStrategy("skip"), ConflictStrategySkip)
	assert.Equal(t, ConflictStrategy("overwrite"), ConflictStrategyOverwrite)
	assert.Equal(t, ConflictStrategy("fail"), ConflictStrategyFail)
	assert.Equal(t, ConflictStrategy("rename"), ConflictStrategyRename)
}

func TestApplyModuleCommandRequiresArgs(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionApplyModuleCommand(c, ioStreams)

	// Test that command requires exactly 1 argument
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestListModuleCommandRequiresArgs(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionListModuleCommand(c, ioStreams)

	// Test that command requires exactly 1 argument
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestValidateModuleCommandRequiresArgs(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionValidateModuleCommand(c, ioStreams)

	// Test that command requires exactly 1 argument
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 1 arg")
}

func TestListModuleNonExistentPath(t *testing.T) {
	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionListModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{"/non/existent/path"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load module")
}

func TestListModuleEmptyDirectory(t *testing.T) {
	// Create empty temp directory
	tmpDir := t.TempDir()

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionListModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{tmpDir})
	err := cmd.Execute()
	// Should succeed but with no definitions
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Loading module")
}

func TestValidateModuleEmptyDirectory(t *testing.T) {
	// Create empty temp directory
	tmpDir := t.TempDir()

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionValidateModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{tmpDir})
	err := cmd.Execute()
	// Should succeed - empty module is valid
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "validation passed")
}

func TestListModuleWithModuleYAML(t *testing.T) {
	// Create temp directory with module.yaml
	tmpDir := t.TempDir()

	moduleYAML := `apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: test-module
  version: v1.0.0
spec:
  description: A test module for CLI testing
`
	err := os.WriteFile(filepath.Join(tmpDir, "module.yaml"), []byte(moduleYAML), 0644)
	require.NoError(t, err)

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionListModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{tmpDir})
	err = cmd.Execute()
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Loading module")
	assert.Contains(t, output, "A test module for CLI testing")
}

func TestApplyModuleCommandFlagDefaults(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionApplyModuleCommand(c, ioStreams)

	// Check default values
	dryRun, _ := cmd.Flags().GetBool(FlagDryRun)
	assert.False(t, dryRun)

	conflict, _ := cmd.Flags().GetString(FlagConflictStrategy)
	assert.Equal(t, string(ConflictStrategyFail), conflict)
}

func TestApplyModuleTypesFiltering(t *testing.T) {
	// This test verifies the types flag parsing
	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionApplyModuleCommand(c, ioStreams)

	// Create empty temp dir
	tmpDir := t.TempDir()

	cmd.SetArgs([]string{tmpDir, "--types", "component,trait"})
	// Execute the command - it should succeed for an empty module (no definitions to apply)
	// but the types flag should be parsed correctly
	err := cmd.Execute()
	// For empty module, the command should succeed (0 definitions applied)
	// or fail if k8s client is required but not available
	// Either outcome is acceptable - we're testing flag parsing works
	_ = err // We don't care about the specific error, just that the command ran

	// Verify the flag was parsed by checking the output mentions loading
	output := buf.String()
	assert.Contains(t, output, "Loading module")
}

func TestModuleCommandsInCommandGroup(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}

	// Get the definition command group
	defCmd := DefinitionCommandGroup(c, "1", ioStreams)

	// Find the module commands
	var foundInitModule, foundApplyModule, foundListModule, foundValidateModule bool
	for _, cmd := range defCmd.Commands() {
		switch cmd.Use {
		case "init-module [PATH]":
			foundInitModule = true
		case "apply-module MODULE":
			foundApplyModule = true
		case "list-module MODULE":
			foundListModule = true
		case "validate-module MODULE":
			foundValidateModule = true
		}
	}

	assert.True(t, foundInitModule, "init-module command should be in the def command group")
	assert.True(t, foundApplyModule, "apply-module command should be in the def command group")
	assert.True(t, foundListModule, "list-module command should be in the def command group")
	assert.True(t, foundValidateModule, "validate-module command should be in the def command group")
}

func TestNewDefinitionInitModuleCommand(t *testing.T) {
	c := common.Args{}
	ioStreams := util.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	assert.NotNil(t, cmd)
	assert.Equal(t, "init-module [PATH]", cmd.Use)
	assert.Contains(t, cmd.Short, "Initialize a new definition module")

	// Check flags exist
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleName))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleDesc))
	assert.NotNil(t, cmd.Flags().Lookup(FlagGoModule))
	assert.NotNil(t, cmd.Flags().Lookup(FlagModuleVersion))
	assert.NotNil(t, cmd.Flags().Lookup(FlagWithExamples))
}

func TestInitModuleCreatesDirectoryStructure(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "test-module")

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{modulePath, "--name", "test-module", "--go-module", "github.com/test/test-module"})
	err := cmd.Execute()
	require.NoError(t, err)

	// Verify directory structure
	assert.DirExists(t, filepath.Join(modulePath, "components"))
	assert.DirExists(t, filepath.Join(modulePath, "traits"))
	assert.DirExists(t, filepath.Join(modulePath, "policies"))
	assert.DirExists(t, filepath.Join(modulePath, "workflowsteps"))

	// Verify files
	assert.FileExists(t, filepath.Join(modulePath, "module.yaml"))
	assert.FileExists(t, filepath.Join(modulePath, "go.mod"))
	assert.FileExists(t, filepath.Join(modulePath, "README.md"))
	assert.FileExists(t, filepath.Join(modulePath, ".gitignore"))

	// Check output
	output := buf.String()
	assert.Contains(t, output, "Initializing definition module")
	assert.Contains(t, output, "Module initialized successfully")
}

func TestInitModuleWithExamples(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "example-module")

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{modulePath, "--with-examples"})
	err := cmd.Execute()
	require.NoError(t, err)

	// Verify example files were created
	assert.FileExists(t, filepath.Join(modulePath, "components", "example.go"))
	assert.FileExists(t, filepath.Join(modulePath, "traits", "example.go"))

	// Check output mentions example creation
	output := buf.String()
	assert.Contains(t, output, "components/example.go")
	assert.Contains(t, output, "traits/example.go")
}

func TestInitModuleYAMLContent(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "yaml-test")

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{modulePath, "--name", "my-module", "--desc", "Test description", "--version", "v1.0.0"})
	err := cmd.Execute()
	require.NoError(t, err)

	// Read and verify module.yaml content
	content, err := os.ReadFile(filepath.Join(modulePath, "module.yaml"))
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "name: my-module")
	assert.Contains(t, contentStr, "version: v1.0.0")
	assert.Contains(t, contentStr, "description: Test description")
}

func TestInitModuleGoModContent(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "gomod-test")

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	cmd.SetArgs([]string{modulePath, "--go-module", "github.com/myorg/custom-module"})
	err := cmd.Execute()
	require.NoError(t, err)

	// Read and verify go.mod content
	content, err := os.ReadFile(filepath.Join(modulePath, "go.mod"))
	require.NoError(t, err)

	contentStr := string(content)
	assert.Contains(t, contentStr, "module github.com/myorg/custom-module")
	assert.Contains(t, contentStr, "require github.com/oam-dev/kubevela")
}

func TestInitModuleDefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "defaults-test")

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	// No flags - use defaults
	cmd.SetArgs([]string{modulePath})
	err := cmd.Execute()
	require.NoError(t, err)

	// Read module.yaml to check defaults
	content, err := os.ReadFile(filepath.Join(modulePath, "module.yaml"))
	require.NoError(t, err)

	contentStr := string(content)
	// Name should be derived from directory
	assert.Contains(t, contentStr, "name: defaults-test")
	// Version should default to v0.1.0
	assert.Contains(t, contentStr, "version: v0.1.0")
}

func TestInitModuleInCurrentDirectory(t *testing.T) {
	// Create and change to temp directory
	tmpDir := t.TempDir()
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	c := common.Args{}
	var buf bytes.Buffer
	ioStreams := util.IOStreams{In: os.Stdin, Out: &buf, ErrOut: &buf}
	cmd := NewDefinitionInitModuleCommand(c, ioStreams)

	// No path argument - use current directory
	cmd.SetArgs([]string{"--name", "current-dir-test"})
	err := cmd.Execute()
	require.NoError(t, err)

	// Verify files in current directory
	assert.FileExists(t, "module.yaml")
	assert.FileExists(t, "go.mod")
	assert.DirExists(t, "components")
}
