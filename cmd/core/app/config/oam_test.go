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

func TestOAMConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *OAMConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name:        "Valid default configuration",
			setupConfig: func() *OAMConfig { return NewOAMConfig() },
			wantErr:     false,
		},
		{
			name: "Invalid: empty namespace",
			setupConfig: func() *OAMConfig {
				cfg := NewOAMConfig()
				cfg.SystemDefinitionNamespace = ""
				return cfg
			},
			wantErr: true,
			errMsg:  "system-definition-namespace must not be empty",
		},
		{
			name: "Invalid: non-DNS-1123 compliant namespace",
			setupConfig: func() *OAMConfig {
				cfg := NewOAMConfig()
				cfg.SystemDefinitionNamespace = "Invalid_NS"
				return cfg
			},
			wantErr: true,
			errMsg:  "system-definition-namespace is invalid: ",
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

func TestOAMConfig_AddFlags(t *testing.T) {
	cfg := NewOAMConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	err := fs.Parse([]string{
		"--system-definition-namespace=custom-system",
	})
	require.NoError(t, err)

	assert.Equal(t, "custom-system", cfg.SystemDefinitionNamespace)
}
