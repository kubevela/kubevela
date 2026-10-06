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

func TestResourceConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *ResourceConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid default configuration",
			setupConfig: func() *ResourceConfig { return NewResourceConfig() },
			wantErr:     false,
		},
		{
			name: "Invalid: max dispatch concurrent zero",
			setupConfig: func() *ResourceConfig {
				cfg := NewResourceConfig()
				cfg.MaxDispatchConcurrent = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "max-dispatch-concurrent must be greater than 0",
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

func TestResourceConfig_AddFlags(t *testing.T) {
	cfg := NewResourceConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	err := fs.Parse([]string{
		"--max-dispatch-concurrent=20",
	})
	require.NoError(t, err)

	assert.Equal(t, 20, cfg.MaxDispatchConcurrent)
}
