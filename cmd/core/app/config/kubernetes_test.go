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

func TestKubernetesConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *KubernetesConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid Configuration - Default",
			setupConfig: func() *KubernetesConfig {
				cfg := &KubernetesConfig{QPS: 50, Burst: 100}
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid Configuration - Negative QPS",
			setupConfig: func() *KubernetesConfig {
				cfg := &KubernetesConfig{QPS: -10, Burst: 100}
				return cfg
			},
			wantErr: true,
			errMsg:  "kubernetes client QPS cannot be negative",
		},
		{
			name: "Invalid Configuration - Negative Burst",
			setupConfig: func() *KubernetesConfig {
				cfg := &KubernetesConfig{QPS: 50, Burst: -1}
				return cfg
			},
			wantErr: true,
			errMsg:  "kubernetes client burst cannot be negative",
		},
		{
			name: "Invalid Configuration - Burst Lower Than QPS",
			setupConfig: func() *KubernetesConfig {
				cfg := &KubernetesConfig{QPS: 100, Burst: 50}
				return cfg
			},
			wantErr: true,
			errMsg:  "kubernetes client burst (50) must be greater than or equal to QPS (100) to prevent token bucket starvation",
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
