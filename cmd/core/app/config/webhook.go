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
	"github.com/spf13/pflag"
)

// WebhookConfig contains webhook configuration.
type WebhookConfig struct {
	UseWebhook  bool
	CertDir     string
	WebhookPort int
}

// NewWebhookConfig creates a new WebhookConfig with defaults.
func NewWebhookConfig() *WebhookConfig {
	return &WebhookConfig{
		UseWebhook:  false,
		CertDir:     "/k8s-webhook-server/serving-certs",
		WebhookPort: 9443,
	}
}

// AddFlags registers webhook configuration flags.
func (c *WebhookConfig) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&c.UseWebhook, "use-webhook", c.UseWebhook,
		"Enable Admission Webhook")
	fs.StringVar(&c.CertDir, "webhook-cert-dir", c.CertDir,
		"Admission webhook cert/key dir.")
	fs.IntVar(&c.WebhookPort, "webhook-port", c.WebhookPort,
		"admission webhook listen address")
}
