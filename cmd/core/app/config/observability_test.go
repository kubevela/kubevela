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

func TestObservabilityConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *ObservabilityConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid default configuration",
			setupConfig: func() *ObservabilityConfig { return NewObservabilityConfig() },
			wantErr:     false,
		},
		{
			name: "Invalid: empty metrics addr",
			setupConfig: func() *ObservabilityConfig {
				cfg := NewObservabilityConfig()
				cfg.MetricsAddr = ""
				return cfg
			},
			wantErr: true,
			errMsg:  "metrics-addr must not be empty",
		},
		{
			name: "Invalid: zero log file max size",
			setupConfig: func() *ObservabilityConfig {
				cfg := NewObservabilityConfig()
				cfg.LogFileMaxSize = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "log-file-max-size must be greater than zero",
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

func TestObservabilityConfig_AddFlags(t *testing.T) {
	cfg := NewObservabilityConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	err := fs.Parse([]string{
		"--metrics-addr=:9090",
		"--log-file-path=/var/log/vela.log",
		"--log-file-max-size=2048",
		"--log-debug=true",
		"--dev-logs=true",
	})
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.MetricsAddr)
	assert.Equal(t, "/var/log/vela.log", cfg.LogFilePath)
	assert.Equal(t, uint64(2048), cfg.LogFileMaxSize)
	assert.True(t, cfg.LogDebug)
	assert.True(t, cfg.DevLogs)
}
