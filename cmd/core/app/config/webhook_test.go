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

func TestWebhookConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupConfig func() *WebhookConfig
		wantErr     bool
		errMsg      string
	}{
		{
			name: "Valid default configuration",
			setupConfig: func() *WebhookConfig {
				return NewWebhookConfig()
			},
			wantErr: false,
		},
		{
			name: "Valid configuration with webhook enabled and cert dir",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.UseWebhook = true
				cfg.CertDir = "/tmp/k8s-webhook-server/certs"
				cfg.WebhookPort = 9443
				return cfg
			},
			wantErr: false,
		},
		{
			name: "Invalid configuration: webhook enabled without cert dir",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.UseWebhook = true
				cfg.CertDir = ""
				return cfg
			},
			wantErr: true,
			errMsg:  "webhook-cert-dir must not be empty when use-webhook is true",
		},
		{
			name: "Invalid configuration: webhook enabled with negative port",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.UseWebhook = true
				cfg.CertDir = "/tmp/certs"
				cfg.WebhookPort = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "webhook-port must be greater than zero",
		},
		{
			name: "Invalid configuration: webhook enabled with zero port",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.UseWebhook = true
				cfg.CertDir = "/tmp/certs"
				cfg.WebhookPort = 0
				return cfg
			},
			wantErr: true,
			errMsg:  "webhook-port must be greater than zero",
		},
		{
			name: "Invalid configuration: webhook disabled with negative port",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.UseWebhook = false
				cfg.WebhookPort = -1
				return cfg
			},
			wantErr: true,
			errMsg:  "webhook-port must be greater than zero",
		},
		{
			name: "Invalid configuration: webhook port above 65535",
			setupConfig: func() *WebhookConfig {
				cfg := NewWebhookConfig()
				cfg.WebhookPort = 65536
				return cfg
			},
			wantErr: true,
			errMsg:  "webhook-port must be between 1 and 65535 (inclusive)",
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
