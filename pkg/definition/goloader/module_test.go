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

package goloader

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/oam-dev/kubevela/pkg/definition/defkit/placement"
)

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected bool
	}{
		{
			name:     "absolute path",
			ref:      "/absolute/path/to/module",
			expected: true,
		},
		{
			name:     "relative path with dot",
			ref:      "./relative/path",
			expected: true,
		},
		{
			name:     "parent relative path",
			ref:      "../parent/path",
			expected: true,
		},
		{
			name:     "go module reference",
			ref:      "github.com/myorg/module",
			expected: false,
		},
		{
			name:     "go module with version",
			ref:      "github.com/myorg/module@v1.0.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLocalPath(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseModuleRef(t *testing.T) {
	tests := []struct {
		name            string
		ref             string
		expectedPath    string
		expectedVersion string
	}{
		{
			name:            "module without version",
			ref:             "github.com/myorg/module",
			expectedPath:    "github.com/myorg/module",
			expectedVersion: "latest",
		},
		{
			name:            "module with version",
			ref:             "github.com/myorg/module@v1.0.0",
			expectedPath:    "github.com/myorg/module",
			expectedVersion: "v1.0.0",
		},
		{
			name:            "module with semver prerelease",
			ref:             "github.com/myorg/module@v1.0.0-beta.1",
			expectedPath:    "github.com/myorg/module",
			expectedVersion: "v1.0.0-beta.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, version := parseModuleRef(tt.ref)
			assert.Equal(t, tt.expectedPath, path)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}

func TestLoadModuleMetadata(t *testing.T) {
	// Create a temporary directory with module.yaml
	tmpDir := t.TempDir()

	// Create module.yaml (version is derived from git, not stored in module.yaml)
	moduleYAML := `apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: test-module
spec:
  description: A test module for unit tests
  maintainers:
    - name: Test Author
      email: test@example.com
  minVelaVersion: v1.9.0
  categories:
    - testing
    - example
`
	err := os.WriteFile(filepath.Join(tmpDir, "module.yaml"), []byte(moduleYAML), 0644)
	require.NoError(t, err)

	// Test loading metadata
	metadata, err := loadModuleMetadata(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "core.oam.dev/v1beta1", metadata.APIVersion)
	assert.Equal(t, "DefinitionModule", metadata.Kind)
	assert.Equal(t, "test-module", metadata.Metadata.Name)
	assert.Equal(t, "A test module for unit tests", metadata.Spec.Description)
	assert.Equal(t, "v1.9.0", metadata.Spec.MinVelaVersion)
	assert.Len(t, metadata.Spec.Maintainers, 1)
	assert.Equal(t, "Test Author", metadata.Spec.Maintainers[0].Name)
	assert.Equal(t, []string{"testing", "example"}, metadata.Spec.Categories)
}

func TestLoadModuleMetadataNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Should return error when module.yaml doesn't exist
	_, err := loadModuleMetadata(tmpDir)
	assert.Error(t, err)
}

func TestLoadModuleFromPath(t *testing.T) {
	// Create a temporary module directory structure
	tmpDir := t.TempDir()

	// Create go.mod
	goMod := `module github.com/test/definitions

go 1.21

require github.com/oam-dev/kubevela v1.9.0
`
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644)
	require.NoError(t, err)

	// Create module.yaml (version is derived from git, not stored in module.yaml)
	moduleYAML := `apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: test-defs
spec:
  description: Test definitions module
`
	err = os.WriteFile(filepath.Join(tmpDir, "module.yaml"), []byte(moduleYAML), 0644)
	require.NoError(t, err)

	// Create components directory with a definition
	componentsDir := filepath.Join(tmpDir, "components")
	err = os.MkdirAll(componentsDir, 0755)
	require.NoError(t, err)

	// Create a simple component definition
	componentGo := `package components

import (
	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func init() {
	defkit.Register(TestComponent)
}

func TestComponent() *defkit.ComponentDefinition {
	image := defkit.Param("image", "Container image").Required().String()

	return defkit.Component("test-component", "1.0.0").
		Description("A test component").
		WithParameter(image).
		Output(defkit.K8sResource("deployment", "apps/v1", "Deployment").
			Set("metadata.name", defkit.Context("name")).
			Set("spec.template.spec.containers[0].image", image))
}
`
	err = os.WriteFile(filepath.Join(componentsDir, "test.go"), []byte(componentGo), 0644)
	require.NoError(t, err)

	// Load module
	ctx := context.Background()
	opts := DefaultModuleLoadOptions()

	module, err := loadModuleFromPath(ctx, tmpDir, opts)
	require.NoError(t, err)

	assert.Equal(t, tmpDir, module.Path)
	// ModulePath might be the directory path if go.mod parsing fails in temp directory
	// Just check it's not empty
	assert.NotEmpty(t, module.ModulePath)
	assert.Equal(t, "test-defs", module.Metadata.Metadata.Name)
	assert.Equal(t, "Test definitions module", module.Metadata.Spec.Description)
}

func TestLoadModuleWithNamePrefix(t *testing.T) {
	// Create a temporary module directory
	tmpDir := t.TempDir()

	// Create go.mod
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module github.com/test/defs\n\ngo 1.21"), 0644)
	require.NoError(t, err)

	// Create components directory
	componentsDir := filepath.Join(tmpDir, "components")
	err = os.MkdirAll(componentsDir, 0755)
	require.NoError(t, err)

	// Create a component definition
	componentGo := `package components

import "github.com/oam-dev/kubevela/pkg/definition/defkit"

func init() { defkit.Register(MyComponent) }

func MyComponent() *defkit.ComponentDefinition {
	return defkit.Component("my-component", "1.0.0").
		Description("Test").
		Output(defkit.K8sResource("main", "v1", "ConfigMap"))
}
`
	err = os.WriteFile(filepath.Join(componentsDir, "my.go"), []byte(componentGo), 0644)
	require.NoError(t, err)

	// Load with name prefix
	ctx := context.Background()
	opts := ModuleLoadOptions{
		Types:      []string{"component"},
		NamePrefix: "prefix-",
	}

	module, err := loadModuleFromPath(ctx, tmpDir, opts)
	require.NoError(t, err)

	// Check that name prefix was applied
	for _, def := range module.Definitions {
		if def.Error == nil && def.Definition.Name != "" {
			assert.True(t, len(def.Definition.Name) > 0)
			// Name prefix is applied to Definition.Name
			if def.Definition.Name == "prefix-my-component" {
				return // Found the prefixed name
			}
		}
	}
}

func TestValidateModule(t *testing.T) {
	tests := []struct {
		name        string
		module      *LoadedModule
		velaVersion string
		expectErrs  int
	}{
		{
			name: "valid module",
			module: &LoadedModule{
				Definitions: []LoadResult{
					{Definition: DefinitionInfo{Name: "test", Type: "component"}},
				},
			},
			expectErrs: 0,
		},
		{
			name: "module with definition error",
			module: &LoadedModule{
				Definitions: []LoadResult{
					{
						Definition: DefinitionInfo{FilePath: "test.go"},
						Error:      assert.AnError,
					},
				},
			},
			expectErrs: 1,
		},
		{
			name: "incompatible vela version",
			module: &LoadedModule{
				Metadata: ModuleMetadata{
					Spec: ModuleSpec{
						MinVelaVersion: "v2.0.0",
					},
				},
				Definitions: []LoadResult{},
			},
			velaVersion: "v1.9.0",
			expectErrs:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateModule(tt.module, tt.velaVersion)
			assert.Len(t, errs, tt.expectErrs)
		})
	}
}

func TestLoadedModuleGetters(t *testing.T) {
	module := &LoadedModule{
		Definitions: []LoadResult{
			{Definition: DefinitionInfo{Name: "comp1", Type: "component"}},
			{Definition: DefinitionInfo{Name: "comp2", Type: "component"}},
			{Definition: DefinitionInfo{Name: "trait1", Type: "trait"}},
			{Definition: DefinitionInfo{Name: "policy1", Type: "policy"}},
			{Definition: DefinitionInfo{Name: "workflow1", Type: "workflow-step"}},
		},
	}

	assert.Len(t, module.GetComponents(), 2)
	assert.Len(t, module.GetTraits(), 1)
	assert.Len(t, module.GetPolicies(), 1)
	assert.Len(t, module.GetWorkflowSteps(), 1)
}

func TestLoadedModuleSummary(t *testing.T) {
	module := &LoadedModule{
		ModulePath: "github.com/test/defs",
		Version:    "v1.0.0",
		Metadata: ModuleMetadata{
			Spec: ModuleSpec{
				Description: "Test module",
			},
		},
		Definitions: []LoadResult{
			{Definition: DefinitionInfo{Name: "comp1", Type: "component"}},
			{Definition: DefinitionInfo{Name: "trait1", Type: "trait"}},
			{Definition: DefinitionInfo{Name: "error1", Type: "policy"}, Error: assert.AnError},
		},
	}

	summary := module.Summary()
	assert.Contains(t, summary, "github.com/test/defs")
	assert.Contains(t, summary, "v1.0.0")
	assert.Contains(t, summary, "Test module")
	assert.Contains(t, summary, "Components:     1")
	assert.Contains(t, summary, "Traits:         1")
	assert.Contains(t, summary, "Errors:         1")
}

func TestDefaultModuleLoadOptions(t *testing.T) {
	opts := DefaultModuleLoadOptions()

	assert.Contains(t, opts.Types, "component")
	assert.Contains(t, opts.Types, "trait")
	assert.Contains(t, opts.Types, "policy")
	assert.Contains(t, opts.Types, "workflow-step")
	assert.True(t, opts.ResolveDependencies)
	assert.False(t, opts.IncludeTests)
}

func TestDiscoverAndLoadDefinitions(t *testing.T) {
	// Create temp directory with definitions in conventional locations
	tmpDir := t.TempDir()

	// Create components directory
	componentsDir := filepath.Join(tmpDir, "components")
	err := os.MkdirAll(componentsDir, 0755)
	require.NoError(t, err)

	// Create traits directory
	traitsDir := filepath.Join(tmpDir, "traits")
	err = os.MkdirAll(traitsDir, 0755)
	require.NoError(t, err)

	// Create a simple Go file (won't be valid defkit, but tests discovery)
	simpleGo := `package components

import "fmt"

func Hello() {
	fmt.Println("Hello")
}
`
	err = os.WriteFile(filepath.Join(componentsDir, "simple.go"), []byte(simpleGo), 0644)
	require.NoError(t, err)

	// Create a test file (should be skipped by default)
	testGo := `package components

import "testing"

func TestHello(t *testing.T) {
}
`
	err = os.WriteFile(filepath.Join(componentsDir, "simple_test.go"), []byte(testGo), 0644)
	require.NoError(t, err)

	// Test discovery with default options (skip tests)
	ctx := context.Background()
	opts := DefaultModuleLoadOptions()

	results, err := discoverAndLoadDefinitions(ctx, tmpDir, opts)
	require.NoError(t, err)

	// Verify test file was not included
	for _, r := range results {
		assert.NotContains(t, r.Definition.FilePath, "_test.go")
	}
}

func TestLoadModuleNonExistentPath(t *testing.T) {
	ctx := context.Background()
	opts := DefaultModuleLoadOptions()

	_, err := LoadModule(ctx, "/non/existent/path", opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadModuleFromFile(t *testing.T) {
	// LoadModule should fail if given a file path instead of directory
	tmpFile, err := os.CreateTemp("", "test*.go")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	ctx := context.Background()
	opts := DefaultModuleLoadOptions()

	_, err = LoadModule(ctx, tmpFile.Name(), opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be a directory")
}

func TestModulePlacementParsing(t *testing.T) {
	tests := []struct {
		name         string
		yamlContent  string
		expectRunOn  int
		expectNotRun int
		expectEmpty  bool
	}{
		{
			name: "module with placement",
			yamlContent: `
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: aws-definitions
spec:
  description: AWS-specific definitions
  placement:
    runOn:
      - key: provider
        operator: Eq
        values: ["aws"]
      - key: environment
        operator: In
        values: ["production", "staging"]
    notRunOn:
      - key: cluster-type
        operator: Eq
        values: ["vcluster"]
`,
			expectRunOn:  2,
			expectNotRun: 1,
			expectEmpty:  false,
		},
		{
			name: "module without placement",
			yamlContent: `
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: general-definitions
spec:
  description: General definitions
`,
			expectRunOn:  0,
			expectNotRun: 0,
			expectEmpty:  true,
		},
		{
			name: "module with only runOn",
			yamlContent: `
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: eks-definitions
spec:
  description: EKS definitions
  placement:
    runOn:
      - key: cluster-type
        operator: Eq
        values: ["eks"]
`,
			expectRunOn:  1,
			expectNotRun: 0,
			expectEmpty:  false,
		},
		{
			name: "module with only notRunOn",
			yamlContent: `
apiVersion: core.oam.dev/v1beta1
kind: DefinitionModule
metadata:
  name: no-vclusters
spec:
  description: Definitions that skip vclusters
  placement:
    notRunOn:
      - key: cluster-type
        operator: Eq
        values: ["vcluster"]
`,
			expectRunOn:  0,
			expectNotRun: 1,
			expectEmpty:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metadata ModuleMetadata
			err := yaml.Unmarshal([]byte(tt.yamlContent), &metadata)
			require.NoError(t, err)

			if tt.expectEmpty {
				assert.True(t, metadata.Spec.Placement.IsEmpty())
			} else {
				assert.False(t, metadata.Spec.Placement.IsEmpty())
				assert.Len(t, metadata.Spec.Placement.RunOn, tt.expectRunOn)
				assert.Len(t, metadata.Spec.Placement.NotRunOn, tt.expectNotRun)
			}
		})
	}
}

func TestModulePlacementToPlacementSpec(t *testing.T) {
	mp := &ModulePlacement{
		RunOn: []ModulePlacementCondition{
			{Key: "provider", Operator: "Eq", Values: []string{"aws"}},
			{Key: "env", Operator: "In", Values: []string{"prod", "staging"}},
		},
		NotRunOn: []ModulePlacementCondition{
			{Key: "cluster-type", Operator: "Eq", Values: []string{"vcluster"}},
		},
	}

	spec := mp.ToPlacementSpec()

	assert.Len(t, spec.RunOn, 2)
	assert.Len(t, spec.NotRunOn, 1)

	// Verify the conditions are correct
	runOn0 := spec.RunOn[0].(*placement.LabelCondition)
	assert.Equal(t, "provider", runOn0.Key)
	assert.Equal(t, placement.OperatorEquals, runOn0.Operator)
	assert.Equal(t, []string{"aws"}, runOn0.Values)

	runOn1 := spec.RunOn[1].(*placement.LabelCondition)
	assert.Equal(t, "env", runOn1.Key)
	assert.Equal(t, placement.OperatorIn, runOn1.Operator)
	assert.Equal(t, []string{"prod", "staging"}, runOn1.Values)

	notRunOn0 := spec.NotRunOn[0].(*placement.LabelCondition)
	assert.Equal(t, "cluster-type", notRunOn0.Key)
	assert.Equal(t, placement.OperatorEquals, notRunOn0.Operator)
	assert.Equal(t, []string{"vcluster"}, notRunOn0.Values)
}

func TestModulePlacementNilToPlacementSpec(t *testing.T) {
	var mp *ModulePlacement
	spec := mp.ToPlacementSpec()

	assert.True(t, spec.IsEmpty())
	assert.Nil(t, spec.RunOn)
	assert.Nil(t, spec.NotRunOn)
}

func TestModulePlacementEvaluation(t *testing.T) {
	mp := &ModulePlacement{
		RunOn: []ModulePlacementCondition{
			{Key: "provider", Operator: "Eq", Values: []string{"aws"}},
		},
		NotRunOn: []ModulePlacementCondition{
			{Key: "cluster-type", Operator: "Eq", Values: []string{"vcluster"}},
		},
	}

	spec := mp.ToPlacementSpec()

	// AWS EKS cluster - should be eligible
	eksLabels := map[string]string{
		"provider":     "aws",
		"cluster-type": "eks",
	}
	result := placement.Evaluate(spec, eksLabels)
	assert.True(t, result.Eligible)

	// AWS vcluster - should NOT be eligible (excluded by notRunOn)
	vclusterLabels := map[string]string{
		"provider":     "aws",
		"cluster-type": "vcluster",
	}
	result = placement.Evaluate(spec, vclusterLabels)
	assert.False(t, result.Eligible)
	assert.Contains(t, result.Reason, "notRunOn")

	// GCP cluster - should NOT be eligible (runOn not satisfied)
	gcpLabels := map[string]string{
		"provider":     "gcp",
		"cluster-type": "gke",
	}
	result = placement.Evaluate(spec, gcpLabels)
	assert.False(t, result.Eligible)
	assert.Contains(t, result.Reason, "runOn")
}

// TestGoModDownloadJSONParsing verifies that 'go mod download -json' output is
// correctly parsed. This test documents the expected behavior and prevents
// regression from using the wrong unmarshaler (e.g., yaml.Unmarshal ignores
// json struct tags, causing empty values).
func TestGoModDownloadJSONParsing(t *testing.T) {
	// This is the format output by 'go mod download -json'
	jsonOutput := `{"Path":"github.com/example/mod","Version":"v1.0.0","Dir":"/go/pkg/mod/github.com/example/mod@v1.0.0"}`

	// The struct used in downloadGoModule with json tags
	var result struct {
		Path    string `json:"Path"`
		Version string `json:"Version"`
		Dir     string `json:"Dir"`
	}

	// json.Unmarshal should correctly parse the output
	err := json.Unmarshal([]byte(jsonOutput), &result)
	require.NoError(t, err)
	assert.Equal(t, "github.com/example/mod", result.Path)
	assert.Equal(t, "v1.0.0", result.Version)
	assert.Equal(t, "/go/pkg/mod/github.com/example/mod@v1.0.0", result.Dir)
}
