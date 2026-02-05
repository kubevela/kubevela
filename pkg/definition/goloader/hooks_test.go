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
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/pkg/utils/util"
)

func TestHookValidation(t *testing.T) {
	tests := []struct {
		name    string
		hook    Hook
		wantErr string
	}{
		{
			name:    "empty hook fails",
			hook:    Hook{},
			wantErr: "must specify either 'path' or 'script'",
		},
		{
			name: "path only succeeds",
			hook: Hook{
				Path: "hooks/pre-apply/crds",
			},
			wantErr: "",
		},
		{
			name: "script only succeeds",
			hook: Hook{
				Script: "hooks/setup.sh",
			},
			wantErr: "",
		},
		{
			name: "both path and script fails",
			hook: Hook{
				Path:   "hooks/pre-apply/crds",
				Script: "hooks/setup.sh",
			},
			wantErr: "cannot specify both 'path' and 'script'",
		},
		{
			name: "wait with script fails",
			hook: Hook{
				Script: "hooks/setup.sh",
				Wait:   true,
			},
			wantErr: "'wait' is only valid for 'path' hooks",
		},
		{
			name: "wait with path succeeds",
			hook: Hook{
				Path: "hooks/pre-apply/crds",
				Wait: true,
			},
			wantErr: "",
		},
		{
			name: "optional is valid for both",
			hook: Hook{
				Script:   "hooks/setup.sh",
				Optional: true,
			},
			wantErr: "",
		},
		{
			name: "timeout is valid",
			hook: Hook{
				Script:  "hooks/setup.sh",
				Timeout: "1m",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hook.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestModuleHooksMethods(t *testing.T) {
	t.Run("IsEmpty", func(t *testing.T) {
		// nil hooks is empty
		var nilHooks *ModuleHooks
		assert.True(t, nilHooks.IsEmpty())

		// empty hooks is empty
		emptyHooks := &ModuleHooks{}
		assert.True(t, emptyHooks.IsEmpty())

		// hooks with pre-apply is not empty
		withPreApply := &ModuleHooks{
			PreApply: []Hook{{Path: "hooks/pre"}},
		}
		assert.False(t, withPreApply.IsEmpty())

		// hooks with post-apply is not empty
		withPostApply := &ModuleHooks{
			PostApply: []Hook{{Script: "hooks/post.sh"}},
		}
		assert.False(t, withPostApply.IsEmpty())
	})

	t.Run("HasPreApply", func(t *testing.T) {
		var nilHooks *ModuleHooks
		assert.False(t, nilHooks.HasPreApply())

		emptyHooks := &ModuleHooks{}
		assert.False(t, emptyHooks.HasPreApply())

		withPreApply := &ModuleHooks{
			PreApply: []Hook{{Path: "hooks/pre"}},
		}
		assert.True(t, withPreApply.HasPreApply())
	})

	t.Run("HasPostApply", func(t *testing.T) {
		var nilHooks *ModuleHooks
		assert.False(t, nilHooks.HasPostApply())

		emptyHooks := &ModuleHooks{}
		assert.False(t, emptyHooks.HasPostApply())

		withPostApply := &ModuleHooks{
			PostApply: []Hook{{Script: "hooks/post.sh"}},
		}
		assert.True(t, withPostApply.HasPostApply())
	})
}

func TestParseTimeout(t *testing.T) {
	tests := []struct {
		name           string
		timeout        string
		defaultTimeout time.Duration
		expected       time.Duration
	}{
		{
			name:           "empty uses default",
			timeout:        "",
			defaultTimeout: 30 * time.Second,
			expected:       30 * time.Second,
		},
		{
			name:           "valid seconds",
			timeout:        "10s",
			defaultTimeout: 30 * time.Second,
			expected:       10 * time.Second,
		},
		{
			name:           "valid minutes",
			timeout:        "5m",
			defaultTimeout: 30 * time.Second,
			expected:       5 * time.Minute,
		},
		{
			name:           "invalid format uses default",
			timeout:        "invalid",
			defaultTimeout: 30 * time.Second,
			expected:       30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimeout(tt.timeout, tt.defaultTimeout)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetYAMLFiles(t *testing.T) {
	// Create a temp directory with test files
	tmpDir := t.TempDir()

	// Create test files
	files := []string{
		"02-second.yaml",
		"01-first.yml",
		"03-third.yaml",
		"not-yaml.txt",
		"readme.md",
	}
	for _, f := range files {
		err := os.WriteFile(filepath.Join(tmpDir, f), []byte("# test"), 0644)
		require.NoError(t, err)
	}

	// Create a subdirectory (should be ignored)
	err := os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	require.NoError(t, err)

	// Get YAML files
	yamlFiles, err := getYAMLFiles(tmpDir)
	require.NoError(t, err)

	// Should return sorted YAML files only
	assert.Len(t, yamlFiles, 3)
	assert.Equal(t, filepath.Join(tmpDir, "01-first.yml"), yamlFiles[0])
	assert.Equal(t, filepath.Join(tmpDir, "02-second.yaml"), yamlFiles[1])
	assert.Equal(t, filepath.Join(tmpDir, "03-third.yaml"), yamlFiles[2])
}

func TestGetYAMLFilesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	yamlFiles, err := getYAMLFiles(tmpDir)
	require.NoError(t, err)
	assert.Empty(t, yamlFiles)
}

func TestGetYAMLFilesNonExistentDir(t *testing.T) {
	_, err := getYAMLFiles("/nonexistent/path")
	assert.Error(t, err)
}

func TestExecuteHooksEmptyList(t *testing.T) {
	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, "/tmp", "default", false, streams)

	stats, err := executor.ExecuteHooks(context.Background(), "pre-apply", []Hook{})
	assert.NoError(t, err)
	assert.Empty(t, out.String())
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.ResourcesCreated)
	assert.Equal(t, 0, stats.ResourcesUpdated)
}

func TestExecuteScriptHookDryRun(t *testing.T) {
	// Create temp directory with a test script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho 'hello'"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", true, streams)

	hook := Hook{
		Script: "test.sh",
	}

	err = executor.executeScriptHook(context.Background(), hook)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "[dry-run]")
	assert.Contains(t, out.String(), "test.sh")
}

func TestExecuteScriptHookActual(t *testing.T) {
	// Create temp directory with a test script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho 'hello from script'"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "test-ns", false, streams)

	hook := Hook{
		Script: "test.sh",
	}

	err = executor.executeScriptHook(context.Background(), hook)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "hello from script")
}

func TestExecuteScriptHookFailure(t *testing.T) {
	// Create temp directory with a failing script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fail.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\necho 'error' >&2\nexit 1"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", false, streams)

	hook := Hook{
		Script: "fail.sh",
	}

	err = executor.executeScriptHook(context.Background(), hook)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script failed")
}

func TestExecuteScriptHookTimeout(t *testing.T) {
	// Create temp directory with a slow script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "slow.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\nsleep 10"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", false, streams)

	hook := Hook{
		Script:  "slow.sh",
		Timeout: "100ms",
	}

	err = executor.executeScriptHook(context.Background(), hook)
	assert.Error(t, err)
}

func TestExecutePathHookDryRun(t *testing.T) {
	// Create temp directory with YAML manifests
	tmpDir := t.TempDir()
	hooksDir := filepath.Join(tmpDir, "hooks")
	err := os.MkdirAll(hooksDir, 0755)
	require.NoError(t, err)

	// Create a test manifest
	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
data:
  key: value`
	err = os.WriteFile(filepath.Join(hooksDir, "01-configmap.yaml"), []byte(manifest), 0644)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", true, streams)

	hook := Hook{
		Path: "hooks",
	}

	created, updated, err := executor.executePathHook(context.Background(), hook)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "[dry-run]")
	assert.Contains(t, out.String(), "ConfigMap")
	assert.Contains(t, out.String(), "test-cm")
	assert.Equal(t, 1, created) // dry-run counts as created
	assert.Equal(t, 0, updated)
}

func TestExecutePathHookEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	hooksDir := filepath.Join(tmpDir, "empty-hooks")
	err := os.MkdirAll(hooksDir, 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", false, streams)

	hook := Hook{
		Path: "empty-hooks",
	}

	created, updated, err := executor.executePathHook(context.Background(), hook)
	assert.NoError(t, err)
	assert.Contains(t, out.String(), "No YAML files found")
	assert.Equal(t, 0, created)
	assert.Equal(t, 0, updated)
}

func TestValidateHooksInModule(t *testing.T) {
	// Create a temp directory for the module
	tmpDir := t.TempDir()

	// Create valid hook directories
	err := os.MkdirAll(filepath.Join(tmpDir, "hooks", "pre-apply", "crds"), 0755)
	require.NoError(t, err)

	// Create a script file
	err = os.WriteFile(filepath.Join(tmpDir, "hooks", "setup.sh"), []byte("#!/bin/bash\necho hello"), 0755)
	require.NoError(t, err)

	t.Run("valid hooks pass", func(t *testing.T) {
		hooks := []Hook{
			{Path: "hooks/pre-apply/crds"},
			{Script: "hooks/setup.sh"},
		}
		errs := validateHooks("pre-apply", hooks, tmpDir)
		assert.Empty(t, errs)
	})

	t.Run("missing path fails", func(t *testing.T) {
		hooks := []Hook{
			{Path: "hooks/nonexistent"},
		}
		errs := validateHooks("pre-apply", hooks, tmpDir)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "does not exist")
	})

	t.Run("path pointing to file fails", func(t *testing.T) {
		hooks := []Hook{
			{Path: "hooks/setup.sh"}, // This is a file, not a directory
		}
		errs := validateHooks("pre-apply", hooks, tmpDir)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "must be a directory")
	})

	t.Run("missing script fails", func(t *testing.T) {
		hooks := []Hook{
			{Script: "hooks/nonexistent.sh"},
		}
		errs := validateHooks("pre-apply", hooks, tmpDir)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "does not exist")
	})

	t.Run("script pointing to directory fails", func(t *testing.T) {
		hooks := []Hook{
			{Script: "hooks/pre-apply/crds"}, // This is a directory, not a file
		}
		errs := validateHooks("pre-apply", hooks, tmpDir)
		assert.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "must be a file")
	})
}

func TestExecuteHooksWithOptional(t *testing.T) {
	// Create temp directory with a failing script
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "fail.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\nexit 1"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", false, streams)

	t.Run("optional hook failure continues", func(t *testing.T) {
		out.Reset()
		hooks := []Hook{
			{Script: "fail.sh", Optional: true},
		}
		stats, err := executor.ExecuteHooks(context.Background(), "pre-apply", hooks)
		assert.NoError(t, err)
		assert.Contains(t, out.String(), "Warning")
		assert.Contains(t, out.String(), "optional")
		assert.Equal(t, 1, stats.OptionalFailed)
	})

	t.Run("required hook failure stops", func(t *testing.T) {
		hooks := []Hook{
			{Script: "fail.sh", Optional: false},
		}
		_, err := executor.ExecuteHooks(context.Background(), "pre-apply", hooks)
		assert.Error(t, err)
	})
}

func TestIsResourceReady(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		wantReady bool
		wantFound bool
	}{
		{
			name:      "no status returns not found",
			obj:       map[string]interface{}{"metadata": map[string]interface{}{"name": "test"}},
			wantReady: false,
			wantFound: false,
		},
		{
			name: "Ready condition True",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
			wantReady: true,
			wantFound: true,
		},
		{
			name: "Ready condition False",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "False",
						},
					},
				},
			},
			wantReady: false,
			wantFound: true,
		},
		{
			name: "Available condition True",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Available",
							"status": "True",
						},
					},
				},
			},
			wantReady: true,
			wantFound: true,
		},
		{
			name: "Established condition True (CRDs)",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Established",
							"status": "True",
						},
					},
				},
			},
			wantReady: true,
			wantFound: true,
		},
		{
			name: "phase Running",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			wantReady: true,
			wantFound: true,
		},
		{
			name: "phase Pending",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Pending",
				},
			},
			wantReady: false,
			wantFound: true,
		},
		{
			name: "phase Bound (PV)",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Bound",
				},
			},
			wantReady: true,
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, found := isResourceReady(obj)
			assert.Equal(t, tt.wantReady, ready)
			assert.Equal(t, tt.wantFound, found)
		})
	}
}

// TestModuleMetadataWithHooks tests YAML parsing of hooks configuration
func TestModuleMetadataWithHooks(t *testing.T) {
	yamlContent := `
apiVersion: defkit.oam.dev/v1
kind: DefinitionModule
metadata:
  name: test-module
spec:
  description: Test module with hooks
  hooks:
    pre-apply:
      - path: hooks/pre-apply/crds/
        wait: true
      - script: hooks/pre-apply/setup.sh
        timeout: "1m"
    post-apply:
      - path: hooks/post-apply/samples/
        optional: true
`

	var metadata ModuleMetadata
	err := yaml.Unmarshal([]byte(yamlContent), &metadata)
	require.NoError(t, err)

	assert.Equal(t, "test-module", metadata.Metadata.Name)
	assert.NotNil(t, metadata.Spec.Hooks)

	// Pre-apply hooks
	require.Len(t, metadata.Spec.Hooks.PreApply, 2)
	assert.Equal(t, "hooks/pre-apply/crds/", metadata.Spec.Hooks.PreApply[0].Path)
	assert.True(t, metadata.Spec.Hooks.PreApply[0].Wait)
	assert.Equal(t, "hooks/pre-apply/setup.sh", metadata.Spec.Hooks.PreApply[1].Script)
	assert.Equal(t, "1m", metadata.Spec.Hooks.PreApply[1].Timeout)

	// Post-apply hooks
	require.Len(t, metadata.Spec.Hooks.PostApply, 1)
	assert.Equal(t, "hooks/post-apply/samples/", metadata.Spec.Hooks.PostApply[0].Path)
	assert.True(t, metadata.Spec.Hooks.PostApply[0].Optional)
}

// TestModuleMetadataWithoutHooks tests that modules without hooks work
func TestModuleMetadataWithoutHooks(t *testing.T) {
	yamlContent := `
apiVersion: defkit.oam.dev/v1
kind: DefinitionModule
metadata:
  name: test-module
spec:
  description: Test module without hooks
`

	var metadata ModuleMetadata
	err := yaml.Unmarshal([]byte(yamlContent), &metadata)
	require.NoError(t, err)

	assert.Equal(t, "test-module", metadata.Metadata.Name)
	assert.Nil(t, metadata.Spec.Hooks)
	assert.True(t, metadata.Spec.Hooks.IsEmpty())
	assert.False(t, metadata.Spec.Hooks.HasPreApply())
	assert.False(t, metadata.Spec.Hooks.HasPostApply())
}

// TestHookExecutionStats tests the HookExecutionStats structure
func TestHookExecutionStats(t *testing.T) {
	t.Run("empty stats", func(t *testing.T) {
		stats := &HookExecutionStats{}
		assert.Equal(t, time.Duration(0), stats.TotalDuration)
		assert.Empty(t, stats.HookDetails)
		assert.Equal(t, 0, stats.ResourcesCreated)
		assert.Equal(t, 0, stats.ResourcesUpdated)
		assert.Equal(t, 0, stats.OptionalFailed)
	})

	t.Run("with hook details", func(t *testing.T) {
		stats := &HookExecutionStats{
			TotalDuration: 5 * time.Second,
			HookDetails: []HookDetail{
				{Name: "hook1", Duration: 2 * time.Second, Wait: true, ResourcesCreated: 3},
				{Name: "hook2", Duration: 3 * time.Second, Wait: false, ResourcesUpdated: 2},
			},
			ResourcesCreated: 3,
			ResourcesUpdated: 2,
			OptionalFailed:   1,
		}

		assert.Equal(t, 5*time.Second, stats.TotalDuration)
		assert.Len(t, stats.HookDetails, 2)
		assert.Equal(t, "hook1", stats.HookDetails[0].Name)
		assert.True(t, stats.HookDetails[0].Wait)
		assert.Equal(t, 3, stats.HookDetails[0].ResourcesCreated)
		assert.Equal(t, 3, stats.ResourcesCreated)
		assert.Equal(t, 2, stats.ResourcesUpdated)
		assert.Equal(t, 1, stats.OptionalFailed)
	})
}

// TestExecuteHooksWithTiming tests that timing is tracked correctly
func TestExecuteHooksWithTiming(t *testing.T) {
	// Create temp directory with a script that takes some time
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "slow.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/bash\nsleep 0.1\necho 'done'"), 0755)
	require.NoError(t, err)

	var out bytes.Buffer
	streams := util.IOStreams{
		Out:    &out,
		ErrOut: &out,
	}

	executor := NewHookExecutor(nil, tmpDir, "default", false, streams)

	hooks := []Hook{
		{Script: "slow.sh"},
	}

	stats, err := executor.ExecuteHooks(context.Background(), "pre-apply", hooks)
	require.NoError(t, err)

	// Should have tracked timing
	assert.True(t, stats.TotalDuration >= 100*time.Millisecond)
	assert.Len(t, stats.HookDetails, 1)
	assert.Equal(t, "slow.sh", stats.HookDetails[0].Name)
	assert.True(t, stats.HookDetails[0].Duration >= 100*time.Millisecond)
}

// TestHookValidationWithWaitFor tests validation of waitFor field
func TestHookValidationWithWaitFor(t *testing.T) {
	tests := []struct {
		name    string
		hook    Hook
		wantErr string
	}{
		{
			name: "waitFor with wait:true succeeds",
			hook: Hook{
				Path:    "hooks/crds",
				Wait:    true,
				WaitFor: "Established",
			},
			wantErr: "",
		},
		{
			name: "waitFor without wait fails",
			hook: Hook{
				Path:    "hooks/crds",
				Wait:    false,
				WaitFor: "Established",
			},
			wantErr: "'waitFor' requires 'wait: true'",
		},
		{
			name: "waitFor with script fails (wait check first)",
			hook: Hook{
				Script:  "hooks/setup.sh",
				Wait:    true,
				WaitFor: "Ready",
			},
			wantErr: "'wait' is only valid for 'path' hooks", // wait check fails first
		},
		{
			name: "CUE expression in waitFor succeeds",
			hook: Hook{
				Path:    "hooks/deploy",
				Wait:    true,
				WaitFor: "status.replicas == status.readyReplicas",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.hook.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestIsSimpleConditionName tests the pattern matching for condition names
func TestIsSimpleConditionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "Ready", input: "Ready", expected: true},
		{name: "Established", input: "Established", expected: true},
		{name: "Available", input: "Available", expected: true},
		{name: "Progressing", input: "Progressing", expected: true},
		{name: "CustomCondition", input: "CustomCondition", expected: true},
		{name: "CamelCase", input: "CamelCase", expected: true},
		{name: "lowercase fails", input: "ready", expected: false},
		{name: "with spaces fails", input: "Ready Condition", expected: false},
		{name: "with dots fails", input: "status.Ready", expected: false},
		{name: "expression fails", input: "status.replicas == 3", expected: false},
		{name: "quoted fails", input: `"Ready"`, expected: false},
		{name: "empty fails", input: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSimpleConditionName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCheckCondition tests checking for specific condition in resource status
func TestCheckCondition(t *testing.T) {
	tests := []struct {
		name          string
		obj           map[string]interface{}
		conditionName string
		wantReady     bool
		wantErr       bool
	}{
		{
			name: "condition found and True",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Established",
							"status": "True",
						},
					},
				},
			},
			conditionName: "Established",
			wantReady:     true,
			wantErr:       false,
		},
		{
			name: "condition found but False",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "False",
						},
					},
				},
			},
			conditionName: "Ready",
			wantReady:     false,
			wantErr:       false,
		},
		{
			name: "condition not found",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
			conditionName: "Established",
			wantReady:     false,
			wantErr:       false,
		},
		{
			name:          "no status",
			obj:           map[string]interface{}{"metadata": map[string]interface{}{"name": "test"}},
			conditionName: "Ready",
			wantReady:     false,
			wantErr:       false,
		},
		{
			name: "no conditions array",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			conditionName: "Ready",
			wantReady:     false,
			wantErr:       false,
		},
		{
			name: "multiple conditions - match second",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Progressing",
							"status": "True",
						},
						map[string]interface{}{
							"type":   "Available",
							"status": "True",
						},
					},
				},
			},
			conditionName: "Available",
			wantReady:     true,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := checkCondition(obj, tt.conditionName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantReady, ready)
		})
	}
}

// TestEvaluateCUEExpression tests CUE expression evaluation against resources
func TestEvaluateCUEExpression(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		expr      string
		wantReady bool
		wantErr   bool
	}{
		{
			name: "simple equality - true",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"replicas":      int64(3),
					"readyReplicas": int64(3),
				},
			},
			expr:      "status.replicas == status.readyReplicas",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "simple equality - false",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"replicas":      int64(3),
					"readyReplicas": int64(2),
				},
			},
			expr:      "status.replicas == status.readyReplicas",
			wantReady: false,
			wantErr:   false,
		},
		{
			name: "string comparison",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			expr:      `status.phase == "Running"`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "greater than",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"availableReplicas": int64(3),
				},
			},
			expr:      "status.availableReplicas >= 2",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "less than - not ready",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"availableReplicas": int64(1),
				},
			},
			expr:      "status.availableReplicas >= 2",
			wantReady: false,
			wantErr:   false,
		},
		{
			name: "boolean field",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"ready": true,
				},
			},
			expr:      "status.ready",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "boolean field - false",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"ready": false,
				},
			},
			expr:      "status.ready",
			wantReady: false,
			wantErr:   false,
		},
		{
			name: "complex expression with and",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase":    "Running",
					"replicas": int64(3),
				},
			},
			expr:      `status.phase == "Running" && status.replicas > 0`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "invalid expression - syntax error",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			expr:      "status.phase === Running", // Invalid CUE syntax
			wantReady: false,
			wantErr:   true,
		},
		{
			name: "non-boolean result - error",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"replicas": int64(3),
				},
			},
			expr:      "status.replicas", // Returns number, not boolean
			wantReady: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := evaluateCUEExpression(obj, tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantReady, ready)
			}
		})
	}
}

// TestEvaluateWaitFor tests the combined evaluation logic
func TestEvaluateWaitFor(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		waitFor   string
		wantReady bool
		wantErr   bool
	}{
		{
			name: "simple condition name",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
			waitFor:   "Ready",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "CUE expression",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"replicas":      int64(3),
					"readyReplicas": int64(3),
				},
			},
			waitFor:   "status.replicas == status.readyReplicas",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "Established condition for CRDs",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "NamesAccepted",
							"status": "True",
						},
						map[string]interface{}{
							"type":   "Established",
							"status": "True",
						},
					},
				},
			},
			waitFor:   "Established",
			wantReady: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := evaluateWaitFor(obj, tt.waitFor)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantReady, ready)
			}
		})
	}
}

// TestModuleMetadataWithWaitFor tests YAML parsing of hooks with waitFor
func TestModuleMetadataWithWaitFor(t *testing.T) {
	yamlContent := `
apiVersion: defkit.oam.dev/v1
kind: DefinitionModule
metadata:
  name: test-module
spec:
  description: Test module with waitFor
  hooks:
    pre-apply:
      - path: hooks/pre-apply/crds/
        wait: true
        waitFor: Established
      - path: hooks/pre-apply/deploy/
        wait: true
        waitFor: "status.replicas == status.readyReplicas"
`

	var metadata ModuleMetadata
	err := yaml.Unmarshal([]byte(yamlContent), &metadata)
	require.NoError(t, err)

	assert.Equal(t, "test-module", metadata.Metadata.Name)
	require.NotNil(t, metadata.Spec.Hooks)

	// Pre-apply hooks with waitFor
	require.Len(t, metadata.Spec.Hooks.PreApply, 2)

	// First hook - simple condition
	assert.Equal(t, "hooks/pre-apply/crds/", metadata.Spec.Hooks.PreApply[0].Path)
	assert.True(t, metadata.Spec.Hooks.PreApply[0].Wait)
	assert.Equal(t, "Established", metadata.Spec.Hooks.PreApply[0].WaitFor)

	// Second hook - CUE expression
	assert.Equal(t, "hooks/pre-apply/deploy/", metadata.Spec.Hooks.PreApply[1].Path)
	assert.True(t, metadata.Spec.Hooks.PreApply[1].Wait)
	assert.Equal(t, "status.replicas == status.readyReplicas", metadata.Spec.Hooks.PreApply[1].WaitFor)
}

// TestEvaluateCUEExpressionEdgeCases tests edge cases for CUE expression evaluation
func TestEvaluateCUEExpressionEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		expr      string
		wantReady bool
		wantErr   bool
	}{
		{
			name: "nested field access",
			obj: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app": "myapp",
					},
				},
			},
			expr:      `metadata.labels.app == "myapp"`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "OR expression",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Succeeded",
				},
			},
			expr:      `status.phase == "Running" || status.phase == "Succeeded"`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "negation",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			expr:      `status.phase != "Failed"`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "missing field returns error",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			expr:      "status.nonexistent == true",
			wantReady: false,
			wantErr:   true, // CUE will error on missing field
		},
		{
			name: "null/nil handling with default",
			obj: map[string]interface{}{
				"status": map[string]interface{}{},
			},
			expr:      `(status.replicas | 0) >= 0`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "list length check",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{"type": "Ready"},
						map[string]interface{}{"type": "Available"},
					},
				},
			},
			expr:      "len(status.conditions) >= 1",
			wantReady: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := evaluateCUEExpression(obj, tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantReady, ready)
			}
		})
	}
}

// TestEvaluateCUEExpressionRealWorldScenarios tests realistic Kubernetes resource scenarios
func TestEvaluateCUEExpressionRealWorldScenarios(t *testing.T) {
	tests := []struct {
		name      string
		obj       map[string]interface{}
		expr      string
		wantReady bool
		wantErr   bool
	}{
		{
			name: "Deployment fully ready",
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "my-deploy",
				},
				"status": map[string]interface{}{
					"replicas":           int64(3),
					"readyReplicas":      int64(3),
					"availableReplicas":  int64(3),
					"updatedReplicas":    int64(3),
					"observedGeneration": int64(2),
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Available",
							"status": "True",
						},
						map[string]interface{}{
							"type":   "Progressing",
							"status": "True",
						},
					},
				},
			},
			expr:      "status.replicas == status.readyReplicas && status.availableReplicas == status.replicas",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "Deployment partially ready",
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"status": map[string]interface{}{
					"replicas":          int64(3),
					"readyReplicas":     int64(2),
					"availableReplicas": int64(2),
				},
			},
			expr:      "status.replicas == status.readyReplicas",
			wantReady: false,
			wantErr:   false,
		},
		{
			name: "StatefulSet ready",
			obj: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "StatefulSet",
				"status": map[string]interface{}{
					"replicas":        int64(3),
					"readyReplicas":   int64(3),
					"currentReplicas": int64(3),
				},
			},
			expr:      "status.readyReplicas >= status.replicas",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "Job completed",
			obj: map[string]interface{}{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"status": map[string]interface{}{
					"succeeded": int64(1),
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Complete",
							"status": "True",
						},
					},
				},
			},
			expr:      "status.succeeded >= 1",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "CRD established",
			obj: map[string]interface{}{
				"apiVersion": "apiextensions.k8s.io/v1",
				"kind":       "CustomResourceDefinition",
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "NamesAccepted",
							"status": "True",
						},
						map[string]interface{}{
							"type":   "Established",
							"status": "True",
						},
					},
				},
			},
			expr:      "status.conditions[1].type == \"Established\" && status.conditions[1].status == \"True\"",
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "Pod running and ready",
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"status": map[string]interface{}{
					"phase": "Running",
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
			expr:      `status.phase == "Running"`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "Service has ClusterIP",
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"spec": map[string]interface{}{
					"clusterIP": "10.0.0.1",
					"type":      "ClusterIP",
				},
			},
			expr:      `spec.clusterIP != ""`,
			wantReady: true,
			wantErr:   false,
		},
		{
			name: "PVC bound",
			obj: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "PersistentVolumeClaim",
				"status": map[string]interface{}{
					"phase": "Bound",
				},
			},
			expr:      `status.phase == "Bound"`,
			wantReady: true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := evaluateCUEExpression(obj, tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantReady, ready)
			}
		})
	}
}

// TestCheckConditionEdgeCases tests edge cases for condition checking
func TestCheckConditionEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		obj           map[string]interface{}
		conditionName string
		wantReady     bool
		wantErr       bool
	}{
		{
			name: "condition with Unknown status",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "Ready",
							"status": "Unknown",
						},
					},
				},
			},
			conditionName: "Ready",
			wantReady:     false, // Unknown is not True
			wantErr:       false,
		},
		{
			name: "empty conditions array",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{},
				},
			},
			conditionName: "Ready",
			wantReady:     false,
			wantErr:       false,
		},
		{
			name: "malformed condition entry",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						"not a map", // Invalid condition
						map[string]interface{}{
							"type":   "Ready",
							"status": "True",
						},
					},
				},
			},
			conditionName: "Ready",
			wantReady:     true, // Should skip invalid and find Ready
			wantErr:       false,
		},
		{
			name: "case sensitive condition name",
			obj: map[string]interface{}{
				"status": map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"type":   "ready", // lowercase
							"status": "True",
						},
					},
				},
			},
			conditionName: "Ready", // Capitalized
			wantReady:     false,   // Case sensitive match
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: tt.obj}
			ready, err := checkCondition(obj, tt.conditionName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantReady, ready)
		})
	}
}
