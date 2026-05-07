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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplicationConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *ApplicationConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid default configuration",
			setupConfig: func() *ApplicationConfig {
				return NewApplicationConfig()
			},
			wantErr: false,
		},
		{
			name: "Valid custom resync period",
			setupConfig: func() *ApplicationConfig {
				cfg := NewApplicationConfig()
				cfg.ReSyncPeriod = 5 * time.Minute
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid negative resync period",
			setupConfig: func() *ApplicationConfig {
				cfg := NewApplicationConfig()
				cfg.ReSyncPeriod = -1 * time.Minute
				return cfg
			},
			wantErr: true,
			errMsg:  "application-re-sync-period must be greater than zero",
		},
		{
			name: "Invalid zero resync period",
			setupConfig: func() *ApplicationConfig {
				cfg := NewApplicationConfig()
				cfg.ReSyncPeriod = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "application-re-sync-period must be greater than zero",
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
