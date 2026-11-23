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

package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestCreateOrUpdateReplacesListsExactly(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	live := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "team-a"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "edit"},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: "alice"},
			{Kind: rbacv1.ServiceAccountKind, Name: "deployer", Namespace: "team-a"},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).Build()

	// Revoking alice leaves the service account, which has no apiGroup, in alice's slot.
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "team-a"},
		RoleRef:    live.RoleRef,
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "deployer", Namespace: "team-a"}},
	}
	res, err := CreateOrUpdate(context.Background(), cli, desired)
	require.NoError(t, err)
	require.Equal(t, controllerutil.OperationResultUpdated, res)

	got := &rbacv1.RoleBinding{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(desired), got))
	require.Equal(t, []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "deployer", Namespace: "team-a"}}, got.Subjects)
}

// The metadata the desired object does not set is left as it is: the API server's, and what other
// controllers added. Labels and annotations are merged; lists elsewhere are replaced exactly.
func TestCreateOrUpdateKeepsMetadataItDoesNotSet(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	created := metav1.NewTime(metav1.Now().Add(-time.Hour).Truncate(time.Second))
	live := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{
		Name: "r", Namespace: "team-a", UID: "live-uid", CreationTimestamp: created,
		Labels:          map[string]string{"owner": "other-controller"},
		Annotations:     map[string]string{"note": "kept"},
		Finalizers:      []string{"example.com/cleanup"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "parent", UID: "parent-uid"}},
	}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).Build()

	desired := func() *rbacv1.Role {
		return &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "team-a", Labels: map[string]string{"app": "shop"}},
			Rules:      []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
		}
	}
	_, err := CreateOrUpdate(context.Background(), cli, desired())
	require.NoError(t, err)

	got := &rbacv1.Role{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(live), got))
	require.Equal(t, types.UID("live-uid"), got.UID)
	require.True(t, created.Equal(&got.CreationTimestamp), "creation time kept")
	require.Equal(t, map[string]string{"owner": "other-controller", "app": "shop"}, got.Labels)
	require.Equal(t, map[string]string{"note": "kept"}, got.Annotations)
	require.Equal(t, []string{"example.com/cleanup"}, got.Finalizers)
	require.Equal(t, live.OwnerReferences, got.OwnerReferences)
	require.Equal(t, desired().Rules, got.Rules)

	res, err := CreateOrUpdate(context.Background(), cli, desired())
	require.NoError(t, err)
	require.Equal(t, controllerutil.OperationResultNone, res, "an unchanged object is not written again")

	clearing := desired()
	clearing.Finalizers, clearing.OwnerReferences = []string{}, []metav1.OwnerReference{}
	_, err = CreateOrUpdate(context.Background(), cli, clearing)
	require.NoError(t, err)
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(live), got))
	require.Empty(t, got.Finalizers, "an empty list clears them")
	require.Empty(t, got.OwnerReferences, "an empty list clears them")
}

func TestCreateOrUpdateUnstructured(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	newCM := func(data map[string]interface{}) *unstructured.Unstructured {
		u := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": "cm", "namespace": "team-a"},
			"data":     data,
		}}
		return u
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newCM(map[string]interface{}{"a": "1", "b": "2"})).Build()

	_, err := CreateOrUpdate(context.Background(), cli, newCM(map[string]interface{}{"a": "3"}))
	require.NoError(t, err)

	got := &unstructured.Unstructured{}
	got.SetAPIVersion("v1")
	got.SetKind("ConfigMap")
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Namespace: "team-a", Name: "cm"}, got))
	data, _, _ := unstructured.NestedStringMap(got.Object, "data")
	require.Equal(t, map[string]string{"a": "3"}, data)
}

// An object being deleted keeps its whole deletion metadata, the grace period with the timestamp.
func TestCreateOrUpdateKeepsTheDeletionGracePeriod(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	deleting := metav1.NewTime(metav1.Now().Truncate(time.Second))
	grace := int64(30)
	live := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "team-a", Finalizers: []string{"example.com/cleanup"},
		DeletionTimestamp: &deleting, DeletionGracePeriodSeconds: &grace}}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(live).Build()

	_, err := CreateOrUpdate(context.Background(), cli, &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "team-a"},
		Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}})
	require.NoError(t, err)
	got := &rbacv1.Role{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKeyFromObject(live), got))
	require.NotNil(t, got.DeletionGracePeriodSeconds)
	require.Equal(t, grace, *got.DeletionGracePeriodSeconds)
}
