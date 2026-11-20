/*
Copyright 2026 The KubeVela Authors.

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

package core_oam_dev

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
)

// webhookPolicies renders the chart's webhook configurations with values and returns each
// webhook's failurePolicy, in order.
func webhookPolicies(t *testing.T, values map[string]interface{}) []string {
	t.Helper()
	chart, err := loader.Load("../../../charts/vela-core")
	require.NoError(t, err)
	vals, err := chartutil.ToRenderValues(chart, values, chartutil.ReleaseOptions{Name: "kubevela", Namespace: "vela-system", IsInstall: true}, chartutil.DefaultCapabilities)
	require.NoError(t, err)
	out, err := engine.Render(chart, vals)
	require.NoError(t, err)
	var policies []string
	re := regexp.MustCompile(`(?m)^\s*failurePolicy:\s*(\S+)`)
	for name, doc := range out {
		if !strings.Contains(name, "admission-webhooks/") || !strings.Contains(name, "WebhookConfiguration") {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(doc, -1) {
			policies = append(policies, m[1])
		}
	}
	require.NotEmpty(t, policies, "the chart renders webhook configurations")
	return policies
}

func webhooks(certManager, patch bool) map[string]interface{} {
	return map[string]interface{}{"admissionWebhooks": map[string]interface{}{
		"certManager": map[string]interface{}{"enabled": certManager},
		"patch":       map[string]interface{}{"enabled": patch},
	}}
}

// The chart renders the configured failure policy wherever nothing else would set it. Webhooks
// start as Ignore only where vela-core manages the certificate and sets the policy itself once it
// serves; with cert-manager or a supplied certificate the chart's Fail is what applies.
func TestWebhooksRenderTheConfiguredFailurePolicyPerCertificateMode(t *testing.T) {
	for _, p := range webhookPolicies(t, webhooks(false, true)) {
		require.Equal(t, "Ignore", p, "vela-core manages the certificate: it sets the policy once it serves")
	}
	for _, p := range webhookPolicies(t, webhooks(true, true)) {
		require.Equal(t, "Fail", p, "cert-manager: nothing else sets the policy")
	}
	for _, p := range webhookPolicies(t, webhooks(true, false)) {
		require.Equal(t, "Fail", p)
	}
	for _, p := range webhookPolicies(t, webhooks(false, false)) {
		require.Equal(t, "Fail", p, "a supplied certificate")
	}
}
