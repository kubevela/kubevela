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
	// TLSSecret is the <namespace>/<name> of the Secret that holds the webhook
	// serving cert. If it is set and UseWebhook is true, vela-core creates or
	// renews the cert at startup. vela-core writes the cert to CertDir and sets the caBundle of the
	// webhook configurations.
	TLSSecret string
	// ConfigurationName is the name of the MutatingWebhookConfiguration and the
	// ValidatingWebhookConfiguration whose caBundle vela-core sets. TLSSecret
	// requires it.
	ConfigurationName string
	// FailurePolicy, if not empty, goes to every webhook when the webhook server
	// serves requests.
	FailurePolicy string
	// ConversionCRDs are CRDs whose conversion webhook vela-core points at its
	// webhook Service.
	ConversionCRDs []string
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
	fs.StringVar(&c.TLSSecret, "webhook-tls-secret", c.TLSSecret,
		"The <namespace>/<name> of the Secret that holds the webhook serving cert. Requires --use-webhook. If you set it, vela-core creates or renews the cert at startup. vela-core writes the cert to --webhook-cert-dir and sets the caBundle of --webhook-configuration. If it is empty (the default), cert-manager or another tool must supply the cert.")
	fs.StringVar(&c.ConfigurationName, "webhook-configuration", c.ConfigurationName,
		"The name of the MutatingWebhookConfiguration and the ValidatingWebhookConfiguration. vela-core sets their caBundle. Use with --webhook-tls-secret.")
	fs.StringVar(&c.FailurePolicy, "webhook-failure-policy", c.FailurePolicy,
		"The failurePolicy (Fail or Ignore) that vela-core sets on every webhook when its webhook server serves requests. Use with --webhook-tls-secret. If it is empty, vela-core does not change the failurePolicy.")
	fs.StringSliceVar(&c.ConversionCRDs, "webhook-conversion-crds", c.ConversionCRDs,
		"The CRDs whose conversion webhook vela-core points at its webhook Service. Use with --webhook-tls-secret.")
}
