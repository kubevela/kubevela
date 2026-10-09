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

package cuetest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sschema "k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// testCluster is the cluster the package's cases share, as a run's do.
func testCluster(t *testing.T) *TestCluster {
	t.Helper()
	c, err := execCluster(".")
	require.NoError(t, err)
	return c
}

func TestTestClusterNamespacesAreFresh(t *testing.T) {
	c := testCluster(t)
	a, err := c.NewNamespace()
	require.NoError(t, err)
	b, err := c.NewNamespace()
	require.NoError(t, err)
	require.NotEqual(t, a, b)
}

func TestTestClusterSeedKeepsStatus(t *testing.T) {
	c := testCluster(t)
	ns, err := c.NewNamespace()
	require.NoError(t, err)
	require.NoError(t, c.Seed(ns, []map[string]any{{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "web"},
		"spec": map[string]any{
			"selector": map[string]any{"matchLabels": map[string]any{"app": "web"}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": "web"}},
				"spec":     map[string]any{"containers": []any{map[string]any{"name": "web", "image": "nginx"}}},
			},
		},
		"status": map[string]any{"replicas": int64(1), "readyReplicas": int64(1)},
	}}))
	got := &unstructured.Unstructured{}
	got.SetAPIVersion("apps/v1")
	got.SetKind("Deployment")
	require.NoError(t, c.Client.Get(context.Background(), client.ObjectKey{Namespace: ns, Name: "web"}, got))
	ready, _, _ := unstructured.NestedInt64(got.Object, "status", "readyReplicas")
	require.EqualValues(t, 1, ready, "status is set, as a controller would have")
}

func TestTestClusterHasKubeVelaCRDs(t *testing.T) {
	c := testCluster(t)
	ns, err := c.NewNamespace()
	require.NoError(t, err)
	require.NoError(t, c.Seed(ns, []map[string]any{{
		"apiVersion": "core.oam.dev/v1beta1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": "shop"},
		"spec":       map[string]any{"components": []any{}},
	}}))
}

func TestStartTestClusterWithoutBinaries(t *testing.T) {
	_, err := StartTestCluster(ClusterOptions{Assets: filepath.Join(t.TempDir(), "none")})
	require.ErrorContains(t, err, "step and source tests run against a local API server")
	require.ErrorContains(t, err, "setup-envtest")
}

func TestKubeVelaCRDsAreFoundFromATestPath(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir, ok := findKubeVelaCRDs(filepath.Join(wd, "testdata"))
	require.True(t, ok)
	require.Equal(t, "crds", filepath.Base(dir))
	_, ok = findKubeVelaCRDs(t.TempDir())
	require.False(t, ok)
}

// A kind without a status subresource keeps the status it was created with;
// there is no subresource to set it through.
func TestSeedKeepsStatusWithoutASubresource(t *testing.T) {
	testCluster(t)
	file := writeHookSuite(t, `"seeds a status the kind stores directly": test.#WorkflowStepExec & {
	definition: "publish"
	crds: ["gadgets.yaml"]
	parameter: {name: "mine", data: {}}
	resources: [{apiVersion: "example.dev/v1", kind: "Gadget", metadata: name: "g", status: phase: "Ready"}]
	expect: resources: [{apiVersion: "example.dev/v1", kind: "Gadget", metadata: name: "g", status: phase: "Ready"}]
}
`)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(file), "gadgets.yaml"), []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gadgets.example.dev
spec:
  group: example.dev
  names: {kind: Gadget, plural: gadgets, singular: gadget}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
`), 0o600))
	s := loadOne(t, file)
	require.Empty(t, s.Evaluate(s.Cases, RunOptions{})[0].Failures)
}

// A CRD a hook or step deleted is back for the next case that lists it.
func TestCRDsAreReinstalledAfterDeletion(t *testing.T) {
	cl := testCluster(t)
	dir := t.TempDir()
	crd := filepath.Join(dir, "widgets.yaml")
	require.NoError(t, os.WriteFile(crd, []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.dev
spec:
  group: example.dev
  names: {kind: Widget, plural: widgets, singular: widget}
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        x-kubernetes-preserve-unknown-fields: true
`), 0o600))
	require.NoError(t, cl.installCRDs([]string{crd}))
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(k8sschema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"})
	obj.SetName("widgets.example.dev")
	require.NoError(t, cl.Client.Delete(context.Background(), obj))
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(cl.Client.Get(context.Background(), client.ObjectKeyFromObject(obj), obj.DeepCopy()))
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, cl.installCRDs([]string{crd}))
	require.NoError(t, cl.Client.Get(context.Background(), client.ObjectKeyFromObject(obj), obj.DeepCopy()))
}
