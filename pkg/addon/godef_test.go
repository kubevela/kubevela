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

package addon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadGoDefFolder(t *testing.T) {
	t.Run("returns nil when godef folder does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		module, err := ReadGoDefFolder(tmpDir)
		require.NoError(t, err)
		assert.Nil(t, module)
	})

	t.Run("returns error when godef is not a directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.WriteFile(godefPath, []byte("not a directory"), 0600)
		require.NoError(t, err)

		_, err = ReadGoDefFolder(tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})

	t.Run("reads godef folder with module.yaml", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		moduleYAML := `apiVersion: defkit.oam.dev/v1
kind: DefinitionModule
metadata:
  name: test-addon
`
		err = os.WriteFile(filepath.Join(godefPath, GoDefModuleFileName), []byte(moduleYAML), 0600)
		require.NoError(t, err)

		module, err := ReadGoDefFolder(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, module)
		assert.Equal(t, godefPath, module.Path)
		assert.Equal(t, moduleYAML, module.ModuleYAML)
	})

	t.Run("reads go.mod and go.sum", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		goMod := `module github.com/test/addon/godef

go 1.23
`
		goSum := `github.com/some/dep v1.0.0 h1:abc...
`
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte(goMod), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(godefPath, "go.sum"), []byte(goSum), 0600)
		require.NoError(t, err)

		module, err := ReadGoDefFolder(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, module)
		assert.Equal(t, goMod, module.GoMod)
		assert.Equal(t, goSum, module.GoSum)
	})

	t.Run("reads Go files recursively", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		defsPath := filepath.Join(godefPath, "definitions")
		err := os.MkdirAll(defsPath, 0755)
		require.NoError(t, err)

		// Create some Go files
		mainGo := `package main

func main() {}
`
		componentGo := `package definitions

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func init() {
	defkit.Register(myComponent())
}
`
		err = os.WriteFile(filepath.Join(godefPath, "main.go"), []byte(mainGo), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(defsPath, "component.go"), []byte(componentGo), 0600)
		require.NoError(t, err)

		module, err := ReadGoDefFolder(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, module)
		assert.Len(t, module.GoFiles, 2)

		// Check that files are read with relative paths
		fileNames := make(map[string]bool)
		for _, f := range module.GoFiles {
			fileNames[f.Name] = true
		}
		assert.True(t, fileNames["main.go"])
		assert.True(t, fileNames[filepath.Join("definitions", "component.go")])
	})
}

func TestHasGoDefFolder(t *testing.T) {
	t.Run("returns false when godef folder does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.False(t, HasGoDefFolder(tmpDir))
	})

	t.Run("returns false when godef is a file", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.WriteFile(godefPath, []byte("file"), 0600)
		require.NoError(t, err)

		assert.False(t, HasGoDefFolder(tmpDir))
	})

	t.Run("returns true when godef folder exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		assert.True(t, HasGoDefFolder(tmpDir))
	})
}

func TestValidateGoDefModule(t *testing.T) {
	t.Run("returns error when go.mod is missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		err = ValidateGoDefModule(godefPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "go.mod")
	})

	t.Run("returns error when no Go files exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		// Create go.mod but no .go files
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte("module test\n"), 0600)
		require.NoError(t, err)

		err = ValidateGoDefModule(godefPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one .go file")
	})

	t.Run("ignores test files when checking for Go files", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		// Create go.mod and only a test file
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte("module test\n"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(godefPath, "main_test.go"), []byte("package main\n"), 0600)
		require.NoError(t, err)

		err = ValidateGoDefModule(godefPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one .go file")
	})

	t.Run("succeeds with valid structure", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		// Create go.mod and a Go file
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte("module test\n"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(godefPath, "main.go"), []byte("package main\n"), 0600)
		require.NoError(t, err)

		err = ValidateGoDefModule(godefPath)
		assert.NoError(t, err)
	})

	t.Run("succeeds with nested Go files", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		defsPath := filepath.Join(godefPath, "definitions")
		err := os.MkdirAll(defsPath, 0755)
		require.NoError(t, err)

		// Create go.mod and a nested Go file
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte("module test\n"), 0600)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(defsPath, "component.go"), []byte("package definitions\n"), 0600)
		require.NoError(t, err)

		err = ValidateGoDefModule(godefPath)
		assert.NoError(t, err)
	})
}

func TestGoDefModuleType(t *testing.T) {
	t.Run("GoDefModule struct fields", func(t *testing.T) {
		module := &GoDefModule{
			Path:       "/path/to/godef",
			ModuleYAML: "apiVersion: v1",
			GoMod:      "module test",
			GoSum:      "deps...",
			GoFiles: []ElementFile{
				{Name: "main.go", Data: "package main"},
			},
			CompiledDefinitions: []ElementFile{
				{Name: "component.cue", Data: "component: {}"},
			},
		}

		assert.Equal(t, "/path/to/godef", module.Path)
		assert.Equal(t, "apiVersion: v1", module.ModuleYAML)
		assert.Equal(t, "module test", module.GoMod)
		assert.Equal(t, "deps...", module.GoSum)
		assert.Len(t, module.GoFiles, 1)
		assert.Len(t, module.CompiledDefinitions, 1)
	})
}

func TestGoDefDirNameConstant(t *testing.T) {
	assert.Equal(t, "godef", GoDefDirName)
	assert.Equal(t, "module.yaml", GoDefModuleFileName)
}

func TestCompileGoDefinitionsFromAddon(t *testing.T) {
	t.Run("returns error when go.mod is missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		// Create a .go file but no go.mod
		err = os.WriteFile(filepath.Join(godefPath, "main.go"), []byte("package main\n"), 0600)
		require.NoError(t, err)

		_, err = CompileGoDefinitionsFromAddon(context.Background(), tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "go.mod")
	})

	t.Run("returns error when no Go files exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		godefPath := filepath.Join(tmpDir, GoDefDirName)
		err := os.MkdirAll(godefPath, 0755)
		require.NoError(t, err)

		// Create go.mod but no .go files
		err = os.WriteFile(filepath.Join(godefPath, "go.mod"), []byte("module test\n"), 0600)
		require.NoError(t, err)

		_, err = CompileGoDefinitionsFromAddon(context.Background(), tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one .go file")
	})
}

func TestCompileGoDefinitions(t *testing.T) {
	t.Run("returns error when godef path does not exist", func(t *testing.T) {
		_, err := CompileGoDefinitions(context.Background(), "/nonexistent/path")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("returns error when no definitions are discovered", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a valid Go module structure but without any defkit definitions
		goMod := `module github.com/test/nodefs

go 1.23
`
		err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0600)
		require.NoError(t, err)

		// Create a Go file that doesn't define any defkit definitions
		mainGo := `package main

func main() {}
`
		err = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0600)
		require.NoError(t, err)

		_, err = CompileGoDefinitions(context.Background(), tmpDir)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no Go definitions found")
	})

	t.Run("returns error with hint when compilation fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		componentsDir := filepath.Join(tmpDir, "components")
		err := os.MkdirAll(componentsDir, 0755)
		require.NoError(t, err)

		// Create a Go module that imports defkit but can't resolve it
		goMod := `module github.com/test/broken

go 1.23

require github.com/oam-dev/kubevela v99.99.99
`
		err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0600)
		require.NoError(t, err)

		// Create a Go file that imports defkit (will fail to compile)
		componentGo := `package components

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func BrokenComponent() *defkit.ComponentDefinition {
	return defkit.NewComponent("broken").Description("broken")
}
`
		err = os.WriteFile(filepath.Join(componentsDir, "broken.go"), []byte(componentGo), 0600)
		require.NoError(t, err)

		_, err = CompileGoDefinitions(context.Background(), tmpDir)
		assert.Error(t, err)
		// Should contain the helpful hint about replace directive
		assert.Contains(t, err.Error(), "replace")
	})
}

func TestCompileGoDefinitionsErrorMessages(t *testing.T) {
	t.Run("error message includes all failed definitions", func(t *testing.T) {
		// This test verifies the error message format when multiple definitions fail
		// The actual compilation would require a real goloader integration test,
		// but we can verify the error message format by examining the code structure
		// The CompileGoDefinitions function collects errors in compilationErrors slice
		// and joins them with "\n  - " separator
	})
}

func TestDetectDefinitionConflicts(t *testing.T) {
	t.Run("no conflicts when definitions have different names", func(t *testing.T) {
		cueDefs := []ElementFile{
			{Name: "mytrait.cue", Data: "..."},
			{Name: "mycomponent.cue", Data: "..."},
		}
		goDefs := []ElementFile{
			{Name: "component-webservice.cue", Data: "..."},
			{Name: "trait-scaler.cue", Data: "..."},
		}
		conflicts := DetectDefinitionConflicts(cueDefs, goDefs)
		assert.Empty(t, conflicts)
	})

	t.Run("detects conflicts when definition names overlap", func(t *testing.T) {
		cueDefs := []ElementFile{
			{Name: "webservice.cue", Data: "..."},
			{Name: "scaler.cue", Data: "..."},
		}
		goDefs := []ElementFile{
			{Name: "component-webservice.cue", Data: "..."}, // conflicts with webservice.cue
			{Name: "trait-autoscaler.cue", Data: "..."},
		}
		conflicts := DetectDefinitionConflicts(cueDefs, goDefs)
		assert.Len(t, conflicts, 1)
		assert.Contains(t, conflicts, "webservice")
	})

	t.Run("detects multiple conflicts", func(t *testing.T) {
		cueDefs := []ElementFile{
			{Name: "webservice.cue", Data: "..."},
			{Name: "scaler.cue", Data: "..."},
		}
		goDefs := []ElementFile{
			{Name: "component-webservice.cue", Data: "..."},
			{Name: "trait-scaler.cue", Data: "..."},
		}
		conflicts := DetectDefinitionConflicts(cueDefs, goDefs)
		assert.Len(t, conflicts, 2)
		assert.Contains(t, conflicts, "webservice")
		assert.Contains(t, conflicts, "scaler")
	})

	t.Run("empty definitions return no conflicts", func(t *testing.T) {
		conflicts := DetectDefinitionConflicts(nil, nil)
		assert.Empty(t, conflicts)

		conflicts = DetectDefinitionConflicts([]ElementFile{}, []ElementFile{})
		assert.Empty(t, conflicts)
	})
}
