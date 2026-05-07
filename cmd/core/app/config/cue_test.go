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

func TestCUEConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *CUEConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid default configuration",
			setupConfig: func() *CUEConfig {
				return NewCUEConfig()
			},
			wantErr: false,
		},
		{
			name: "Valid configuration with external package enabled and watch enabled",
			setupConfig: func() *CUEConfig {
				cfg := NewCUEConfig()
				cfg.EnableExternalPackage = true
				cfg.EnableExternalPackageWatch = true
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Valid configuration with external package enabled and watch disabled",
			setupConfig: func() *CUEConfig {
				cfg := NewCUEConfig()
				cfg.EnableExternalPackage = true
				cfg.EnableExternalPackageWatch = false
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid configuration: external package disabled but watch enabled",
			setupConfig: func() *CUEConfig {
				cfg := NewCUEConfig()
				cfg.EnableExternalPackage = false
				cfg.EnableExternalPackageWatch = true
				return cfg
			},
			wantErr: true,
			errMsg:  "enable-external-package-watch-for-default-compiler cannot be true when enable-external-package-for-default-compiler is false",
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
