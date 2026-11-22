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

package app

import (
	"context"
	"testing"

	"github.com/kubevela/workflow/pkg/utils/httpguard"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func denyConfigMap(name, cidrs string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vela-system",
			Labels: map[string]string{httpguard.ConfigTemplateLabel: "vela-core-http-deny"}},
		Data: map[string]string{httpguard.ConfigMapKeyDenyCIDRs: cidrs},
	}
}

func deniesCIDR(cidr string) bool {
	for _, n := range httpguard.Current().DenyCIDRs {
		if n.String() == cidr {
			return true
		}
	}
	return false
}

func TestLoadHTTPDenyConfigMapsAtStartup(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { httpguard.SetDenyFragment(httpguard.Policy{}) })

	good := fake.NewClientBuilder().WithObjects(denyConfigMap("extra", "10.9.9.0/24")).Build()
	require.NoError(t, loadHTTPDenyConfigMaps(ctx, good, "vela-core-http-deny", "vela-system"))
	require.True(t, deniesCIDR("10.9.9.0/24"), "a valid ConfigMap's entries are denied")

	// A running controller logs a ConfigMap that does not parse and keeps its last good list; at
	// startup there is no last good list, so it starts with the built-in one instead of refusing to start.
	bad := fake.NewClientBuilder().WithObjects(denyConfigMap("typo", "not-a-cidr")).Build()
	require.NoError(t, loadHTTPDenyConfigMaps(ctx, bad, "vela-core-http-deny", "vela-system"))
	require.False(t, deniesCIDR("10.9.9.0/24"), "nothing from an earlier load survives a failed start")
	for _, n := range httpguard.BuiltinDeny().DenyCIDRs {
		require.True(t, deniesCIDR(n.String()), "the built-in list still applies: %s", n)
	}
}
