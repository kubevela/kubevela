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

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *ServerConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid default configuration",
			setupConfig: func() *ServerConfig {
				return NewServerConfig()
			},
			wantErr: false,
		},
		{
			name: "Valid configuration with leader election enabled",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid configuration: zero lease duration",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.LeaseDuration = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-lease-duration must be greater than zero",
		},
		{
			name: "Invalid configuration: negative lease duration",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.LeaseDuration = -1 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-lease-duration must be greater than zero",
		},
		{
			name: "Invalid configuration: zero renew deadline",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.RenewDeadline = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-renew-deadline must be greater than zero",
		},
		{
			name: "Invalid configuration: negative renew deadline",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.RenewDeadline = -1 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-renew-deadline must be greater than zero",
		},
		{
			name: "Invalid configuration: zero retry period",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.RetryPeriod = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-retry-period must be greater than zero",
		},
		{
			name: "Invalid configuration: negative retry period",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.RetryPeriod = -1 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-retry-period must be greater than zero",
		},
		{
			name: "Invalid configuration: lease duration equal to renew deadline",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.LeaseDuration = 10 * time.Second
				cfg.RenewDeadline = 10 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-lease-duration must be greater than leader-election-renew-deadline",
		},
		{
			name: "Invalid configuration: lease duration less than renew deadline",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.LeaseDuration = 5 * time.Second
				cfg.RenewDeadline = 10 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-lease-duration must be greater than leader-election-renew-deadline",
		},
		{
			name: "Invalid configuration: retry period too close to renew deadline (Jitter)",
			setupConfig: func() *ServerConfig {
				cfg := NewServerConfig()
				cfg.EnableLeaderElection = true
				cfg.LeaseDuration = 15 * time.Second
				cfg.RenewDeadline = 5 * time.Second
				cfg.RetryPeriod = 5 * time.Second
				return cfg
			},
			wantErr: true,
			errMsg:  "leader-election-renew-deadline must be greater than leader-election-retry-period * Jitter (1.2)",
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
