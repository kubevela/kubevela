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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *ControllerConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid default configuration",
			setupConfig: func() *ControllerConfig {
				return NewControllerConfig()
			},
			wantErr: false,
		},
		{
			name: "Invalid concurrent reconciles",
			setupConfig: func() *ControllerConfig {
				cfg := NewControllerConfig()
				cfg.ConcurrentReconciles = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "concurrent-reconciles must be greater than zero",
		},
		{
			name: "Invalid negative revision limit",
			setupConfig: func() *ControllerConfig {
				cfg := NewControllerConfig()
				cfg.RevisionLimit = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "revision-limit must be greater than or equal to zero",
		},
		{
			name: "Invalid negative app revision limit",
			setupConfig: func() *ControllerConfig {
				cfg := NewControllerConfig()
				cfg.AppRevisionLimit = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "application-revision-limit must be greater than or equal to zero",
		},
		{
			name: "Invalid negative def revision limit",
			setupConfig: func() *ControllerConfig {
				cfg := NewControllerConfig()
				cfg.DefRevisionLimit = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "definition-revision-limit must be greater than or equal to zero",
		},
		{
			name: "Valid zero limits",
			setupConfig: func() *ControllerConfig {
				cfg := NewControllerConfig()
				cfg.RevisionLimit = 0
				cfg.AppRevisionLimit = 0
				cfg.DefRevisionLimit = 0
				return cfg
			},
			wantErr: false,
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
