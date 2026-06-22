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

package addon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestTrackerName(t *testing.T) {
	assert.Equal(t, "addon-fluxcd-drift", trackerName("fluxcd"))
}

func TestManagedResourceFrom(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.FromAPIVersionAndKind("apps/v1", "Deployment"))
	obj.SetNamespace("vela-system")
	obj.SetName("x")

	mr, err := managedResourceFrom(obj)
	assert.NoError(t, err)
	assert.Equal(t, "Deployment", mr.Kind)
	assert.Equal(t, "apps/v1", mr.APIVersion)
	assert.Equal(t, "vela-system", mr.Namespace)
	assert.Equal(t, "x", mr.Name)
	assert.NotNil(t, mr.Data)

	// round-trips back to the same object
	got, err := mr.ToUnstructuredWithData()
	assert.NoError(t, err)
	assert.Equal(t, obj.GetName(), got.GetName())
	assert.Equal(t, obj.GetKind(), got.GetKind())

	var raw map[string]interface{}
	assert.NoError(t, json.Unmarshal(mr.Data.Raw, &raw))
}

func TestManagedResourceFromStripsServerFields(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.FromAPIVersionAndKind("apps/v1", "Deployment"))
	obj.SetNamespace("vela-system")
	obj.SetName("x")
	obj.SetResourceVersion("123")
	obj.SetUID("the-uid")
	obj.SetGeneration(7)
	assert.NoError(t, unstructured.SetNestedField(obj.Object, "running", "status", "phase"))

	mr, err := managedResourceFrom(obj)
	assert.NoError(t, err)

	var raw map[string]interface{}
	assert.NoError(t, json.Unmarshal(mr.Data.Raw, &raw))
	meta, _ := raw["metadata"].(map[string]interface{})
	assert.NotContains(t, meta, "resourceVersion")
	assert.NotContains(t, meta, "uid")
	assert.NotContains(t, meta, "generation")
	assert.NotContains(t, meta, "creationTimestamp")
	assert.NotContains(t, raw, "status")
	// identity preserved
	assert.Equal(t, "apps/v1", raw["apiVersion"])
	assert.Equal(t, "Deployment", raw["kind"])
	assert.Equal(t, "x", meta["name"])
}

func TestWriteTrackerEmptyObjectsErrors(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd"}}
	err := r.writeTracker(context.Background(), ad, nil)
	assert.Error(t, err)
	rt, _ := r.loadTracker(context.Background(), "fluxcd")
	assert.Nil(t, rt)
}

func TestWriteAndLoadTracker(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", UID: "u1"}}

	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))
	cm.SetNamespace("vela-system")
	cm.SetName("c1")

	assert.NoError(t, r.writeTracker(context.Background(), ad, []client.Object{cm}))

	rt, err := r.loadTracker(context.Background(), "fluxcd")
	assert.NoError(t, err)
	assert.NotNil(t, rt)
	assert.Len(t, rt.Spec.ManagedResources, 1)
	assert.Equal(t, "c1", rt.Spec.ManagedResources[0].Name)
	assert.Equal(t, trackerName("fluxcd"), rt.Name)
	// owned by the addon
	assert.Len(t, rt.OwnerReferences, 1)
	assert.Equal(t, "fluxcd", rt.OwnerReferences[0].Name)
	// carries the owned-by label and not the application GC labels
	assert.Equal(t, "fluxcd", rt.Labels[trackerOwnedByAddonLabel])
}

func TestLoadTrackerNotFoundReturnsNil(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	rt, err := r.loadTracker(context.Background(), "absent")
	assert.NoError(t, err)
	assert.Nil(t, rt)
}

func TestHealRecreatesDeleted(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	ad := &v1beta1.Addon{ObjectMeta: metav1.ObjectMeta{Name: "fluxcd", UID: "u1"}}

	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))
	cm.SetNamespace("vela-system")
	cm.SetName("c1")
	assert.NoError(t, r.writeTracker(context.Background(), ad, []client.Object{cm}))

	// c1 does not exist in the cluster; heal must recreate it from the tracker.
	rt, _ := r.loadTracker(context.Background(), "fluxcd")
	assert.NoError(t, r.healFromTracker(context.Background(), rt))

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.FromAPIVersionAndKind("v1", "ConfigMap"))
	assert.NoError(t, cli.Get(context.Background(),
		client.ObjectKey{Namespace: "vela-system", Name: "c1"}, got))
}

func TestHealNoTrackerIsNoop(t *testing.T) {
	scheme := newTestScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{Client: cli, Scheme: scheme}
	assert.NoError(t, r.healFromTracker(context.Background(), nil))
}
