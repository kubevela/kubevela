/*
 Copyright 2026. The KubeVela Authors.

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

package query

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	querytypes "github.com/oam-dev/kubevela/pkg/utils/types"
)

// A Secret is reported by name and metadata, never its data: a query runs with the controller's
// rights, and a resource tree reaches Secrets its caller may not read.
func TestACollectedSecretCarriesNoData(t *testing.T) {
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": "token", "namespace": "victim", "labels": map[string]interface{}{"a": "b"},
			"annotations": map[string]interface{}{"kubectl.kubernetes.io/last-applied-configuration": "{\"data\":{\"token\":\"c2VjcmV0\"}}", "app.oam.dev/last-applied-configuration": "{\"data\":{\"token\":\"c2VjcmV0\"}}", "keep": "me"}},
		"data":       map[string]interface{}{"token": "c2VjcmV0"},
		"stringData": map[string]interface{}{"password": "secret"},
		"type":       "Opaque",
	}}
	parent := &querytypes.ResourceTreeNode{APIVersion: "helm.toolkit.fluxcd.io/v2", Kind: "HelmRelease", Name: "r", Namespace: "team-a"}
	node := &querytypes.ResourceTreeNode{APIVersion: "v1", Kind: "Secret", Name: "token", Namespace: "victim", Object: secret}

	items := buildResourceArray(querytypes.AppliedResource{}, parent, node, "Secret", "v1")
	require.Len(t, items, 1)
	got := items[0].Object
	require.NotContains(t, got.Object, "data")
	require.NotContains(t, got.Object, "stringData")
	require.NotContains(t, got.GetAnnotations(), "kubectl.kubernetes.io/last-applied-configuration")
	require.NotContains(t, got.GetAnnotations(), "app.oam.dev/last-applied-configuration")
	require.Equal(t, "me", got.GetAnnotations()["keep"])
	require.Equal(t, "Opaque", got.Object["type"])
	require.Equal(t, map[string]string{"a": "b"}, got.GetLabels())
	require.Contains(t, secret.Object, "data", "the tree's own copy is not changed")

	pod := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": "p"}, "data": map[string]interface{}{"x": "y"}}}
	items = buildResourceArray(querytypes.AppliedResource{}, parent,
		&querytypes.ResourceTreeNode{APIVersion: "v1", Kind: "Pod", Name: "p", Object: pod}, "Pod", "v1")
	require.Contains(t, items[0].Object.Object, "data", "only Secrets are stripped")
}

// A listed resource is redacted as a collected one is.
func TestAListedSecretCarriesNoData(t *testing.T) {
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": "db", "namespace": "team-a"},
		"data":     map[string]interface{}{"token": "c2VjcmV0"},
	}}
	got := withoutSecretDataIn([]Resource{{Cluster: "local", Object: secret}})
	require.NotContains(t, got[0].Object.Object, "data")
	require.Contains(t, secret.Object, "data", "the collected object itself is left as it is")
}
