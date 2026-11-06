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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	configv1alpha1 "github.com/oam-dev/kubevela/apis/config.oam.dev/v1alpha1"
)

// TestNacosEndpointIndexValue pins the key the endpoint Secret watch looks Configs up by:
// namespace/name of the endpoint the status records, and nothing for a Config whose
// status has no nacos part or no endpoint yet.
func TestNacosEndpointIndexValue(t *testing.T) {
	r := require.New(t)
	r.Nil(nacosEndpointIndexValue(&corev1.Secret{}), "not a Config")
	r.Nil(nacosEndpointIndexValue(&configv1alpha1.Config{}), "no status")
	r.Nil(nacosEndpointIndexValue(&configv1alpha1.Config{Status: configv1alpha1.ConfigStatus{
		ExpandedWriter: &configv1alpha1.ExpandedWriterStatus{Nacos: &configv1alpha1.NacosSyncStatus{DataID: "nc-db"}},
	}}), "synced before the endpoint was recorded")
	r.Equal([]string{"vela-system/nacos"}, nacosEndpointIndexValue(&configv1alpha1.Config{Status: configv1alpha1.ConfigStatus{
		ExpandedWriter: &configv1alpha1.ExpandedWriterStatus{Nacos: &configv1alpha1.NacosSyncStatus{
			DataID:   "nc-db",
			Endpoint: &configv1alpha1.NacosEndpointReference{Name: "nacos", Namespace: "vela-system"},
		}},
	}}))
}
