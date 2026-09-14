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
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiClusterConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *MultiClusterConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid default configuration",
			setupConfig: func() *MultiClusterConfig { return NewMultiClusterConfig() },
			wantErr:     false,
		},
		{
			name: "Valid with both gateway and metrics enabled",
			setupConfig: func() *MultiClusterConfig {
				cfg := NewMultiClusterConfig()
				cfg.EnableClusterGateway = true
				cfg.EnableClusterMetrics = true
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid: metrics enabled without gateway",
			setupConfig: func() *MultiClusterConfig {
				cfg := NewMultiClusterConfig()
				cfg.EnableClusterMetrics = true
				cfg.EnableClusterGateway = false
				return cfg
			},
			wantErr: true,
			errMsg:  "enable-cluster-gateway must be true when enable-cluster-metrics is true",
		},
		{
			name: "Invalid: zero metrics interval",
			setupConfig: func() *MultiClusterConfig {
				cfg := NewMultiClusterConfig()
				cfg.ClusterMetricsInterval = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "cluster-metrics-interval must be greater than zero",
		},
		{
			name: "Invalid: negative metrics interval",
			setupConfig: func() *MultiClusterConfig {
				cfg := NewMultiClusterConfig()
				cfg.ClusterMetricsInterval = -1 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "cluster-metrics-interval must be greater than zero",
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

func TestMultiClusterConfig_AddFlags(t *testing.T) {
	cfg := NewMultiClusterConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	err := fs.Parse([]string{
		"--enable-cluster-gateway=true",
		"--enable-cluster-metrics=true",
		"--cluster-metrics-interval=30s",
	})
	require.NoError(t, err)

	assert.True(t, cfg.EnableClusterGateway)
	assert.True(t, cfg.EnableClusterMetrics)
	assert.Equal(t, 30*time.Second, cfg.ClusterMetricsInterval)
}
