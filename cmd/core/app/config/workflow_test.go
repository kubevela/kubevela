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

package config

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *WorkflowConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid default configuration",
			setupConfig: func() *WorkflowConfig { return NewWorkflowConfig() },
			wantErr:     false,
		},
		{
			name: "Invalid: max wait backoff time zero",
			setupConfig: func() *WorkflowConfig {
				cfg := NewWorkflowConfig()
				cfg.MaxWaitBackoffTime = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "max-workflow-wait-backoff-time must be greater than 0",
		},
		{
			name: "Invalid: max failed backoff time zero",
			setupConfig: func() *WorkflowConfig {
				cfg := NewWorkflowConfig()
				cfg.MaxFailedBackoffTime = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "max-workflow-failed-backoff-time must be greater than 0",
		},
		{
			name: "Invalid: max step error retry times zero",
			setupConfig: func() *WorkflowConfig {
				cfg := NewWorkflowConfig()
				cfg.MaxStepErrorRetryTimes = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "max-workflow-step-error-retry-times must be greater than 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkflowConfig_AddFlags(t *testing.T) {
	cfg := NewWorkflowConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	err := fs.Parse([]string{
		"--max-workflow-wait-backoff-time=120",
		"--max-workflow-failed-backoff-time=600",
		"--max-workflow-step-error-retry-times=20",
	})
	require.NoError(t, err)

	assert.Equal(t, 120, cfg.MaxWaitBackoffTime)
	assert.Equal(t, 600, cfg.MaxFailedBackoffTime)
	assert.Equal(t, 20, cfg.MaxStepErrorRetryTimes)
}
