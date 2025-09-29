/*
Copyright 2021 The KubeVela Authors.

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

package component

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/pkg/features"
)

func TestGetLabelSelectorFromRefObjectSelector(t *testing.T) {
	type args struct {
		selector v1alpha1.ObjectReferrer
	}
	tests := []struct {
		name      string
		args      args
		featureOn bool
		want      map[string]string
	}{
		{
			name: "label selector present",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						LabelSelector: map[string]string{"app": "my-app"},
					},
				},
			},
			want: map[string]string{"app": "my-app"},
		},
		{
			name: "deprecated label selector present and feature on",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						DeprecatedLabelSelector: map[string]string{"app": "my-app-deprecated"},
					},
				},
			},
			featureOn: true,
			want:      map[string]string{"app": "my-app-deprecated"},
		},
		{
			name: "deprecated label selector present and feature off",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						DeprecatedLabelSelector: map[string]string{"app": "my-app-deprecated"},
					},
				},
			},
			featureOn: false,
			want:      nil,
		},
		{
			name: "both present, label selector takes precedence",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						LabelSelector:           map[string]string{"app": "my-app"},
						DeprecatedLabelSelector: map[string]string{"app": "my-app-deprecated"},
					},
				},
			},
			featureOn: true,
			want:      map[string]string{"app": "my-app"},
		},
		{
			name: "no selector",
			args: args{
				selector: v1alpha1.ObjectReferrer{},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.DeprecatedObjectLabelSelector, tt.featureOn)
			if got := GetLabelSelectorFromRefObjectSelector(tt.args.selector); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetLabelSelectorFromRefObjectSelector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateRefObjectSelector(t *testing.T) {
	type args struct {
		selector v1alpha1.ObjectReferrer
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "name only",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						Name: "my-obj",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "label selector only",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						LabelSelector: map[string]string{"app": "my-app"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both name and label selector",
			args: args{
				selector: v1alpha1.ObjectReferrer{
					ObjectSelector: v1alpha1.ObjectSelector{
						Name:          "my-obj",
						LabelSelector: map[string]string{"app": "my-app"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty selector",
			args: args{
				selector: v1alpha1.ObjectReferrer{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateRefObjectSelector(tt.args.selector); (err != nil) != tt.wantErr {
				t.Errorf("ValidateRefObjectSelector() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClearRefObjectForDispatch(t *testing.T) {
	un := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":              "test-obj",
				"namespace":         "default",
				"resourceVersion":   "12345",
				"generation":        int64(1),
				"uid":               "abc-def",
				"creationTimestamp": "2021-01-01T00:00:00Z",
				"managedFields":     []interface{}{},
				"ownerReferences":   []interface{}{},
			},
			"status": map[string]interface{}{
				"phase": "Available",
			},
		},
	}
	ClearRefObjectForDispatch(un)

	if un.GetResourceVersion() != "" {
		t.Errorf("resourceVersion should be cleared")
	}
	if un.GetGeneration() != 0 {
		t.Errorf("generation should be cleared")
	}
	if len(un.GetOwnerReferences()) != 0 {
		t.Errorf("ownerReferences should be cleared")
	}
	if un.GetDeletionTimestamp() != nil {
		t.Errorf("deletionTimestamp should be nil")
	}
	if len(un.GetManagedFields()) != 0 {
		t.Errorf("managedFields should be cleared")
	}
	if un.GetUID() != "" {
		t.Errorf("uid should be cleared")
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(un.Object, "metadata", "creationTimestamp"); found {
		t.Errorf("creationTimestamp should be removed")
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(un.Object, "status"); found {
		t.Errorf("status should be removed")
	}

	// Test for service
	svc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name": "test-svc",
			},
			"spec": map[string]interface{}{
				"clusterIP":  "1.2.3.4",
				"clusterIPs": []interface{}{"1.2.3.4"},
			},
		},
	}
	ClearRefObjectForDispatch(svc)
	if _, found, _ := unstructured.NestedString(svc.Object, "spec", "clusterIP"); found {
		t.Errorf("service clusterIP should be removed")
	}
	if _, found, _ := unstructured.NestedStringSlice(svc.Object, "spec", "clusterIPs"); found {
		t.Errorf("service clusterIPs should be removed")
	}

	// Test for service with ClusterIPNone
	svcNone := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name": "test-svc-none",
			},
			"spec": map[string]interface{}{
				"clusterIP": corev1.ClusterIPNone,
			},
		},
	}
	ClearRefObjectForDispatch(svcNone)
	if ip, found, _ := unstructured.NestedString(svcNone.Object, "spec", "clusterIP"); !found || ip != corev1.ClusterIPNone {
		t.Errorf("service with clusterIP None should not be removed")
	}
}

func TestSelectRefObjectsForDispatch(t *testing.T) {
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.LegacyObjectTypeIdentifier, true)
	featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.DeprecatedObjectLabelSelector, true)
	t.Log("Create objects")
	if err := k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}}); err != nil {
		t.Fatal(err)
	}
	for _, obj := range []client.Object{&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamic",
			Namespace: "test",
		},
	}, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "dynamic",
			Namespace:  "test",
			Generation: int64(5),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.0.0.254",
			Ports:     []corev1.ServicePort{{Port: 80}},
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "by-label-1",
			Namespace: "test",
			Labels:    map[string]string{"key": "value"},
		},
	}, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "by-label-2",
			Namespace: "test",
			Labels:    map[string]string{"key": "value"},
		},
	}, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster-role",
		},
	}} {
		if err := k8sClient.Create(context.Background(), obj); err != nil {
			t.Fatal(err)
		}
	}
	createUnstructured := func(apiVersion string, kind string, name string, namespace string, labels map[string]interface{}) *unstructured.Unstructured {
		un := &unstructured.Unstructured{
			Object: map[string]interface{}{"apiVersion": apiVersion,
				"kind":     kind,
				"metadata": map[string]interface{}{"name": name},
			},
		}
		if namespace != "" {
			un.SetNamespace(namespace)
		}
		if labels != nil {
			un.Object["metadata"].(map[string]interface{})["labels"] = labels
		}
		return un
	}
	testcases := map[string]struct {
		Input         v1alpha1.ObjectReferrer
		compName      string
		appNs         string
		Output        []*unstructured.Unstructured
		Error         string
		Scope         string
		IsService     bool
		IsClusterRole bool
	}{
		"normal": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
			},
			appNs:  "test",
			Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
		},
		"legacy-type-identifier": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{LegacyObjectTypeIdentifier: v1alpha1.LegacyObjectTypeIdentifier{Kind: "ConfigMap", APIVersion: "v1"}},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
			},
			appNs:  "test",
			Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
		},
		"invalid-apiVersion": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{LegacyObjectTypeIdentifier: v1alpha1.LegacyObjectTypeIdentifier{Kind: "ConfigMap", APIVersion: "a/b/v1"}},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
			},
			appNs: "test",
			Error: "invalid APIVersion",
		},
		"invalid-type-identifier": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
			},
			appNs: "test",
			Error: "neither resource or apiVersion/kind is set",
		},
		"name-and-selector-both-set": {
			Input: v1alpha1.ObjectReferrer{ObjectSelector: v1alpha1.ObjectSelector{Name: "dynamic", LabelSelector: map[string]string{"key": "value"}}},
			appNs: "test",
			Error: "invalid object selector for ref-objects, name and labelSelector cannot be both set",
		},
		"empty-ref-object-name": {
			Input:    v1alpha1.ObjectReferrer{ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"}},
			compName: "dynamic",
			appNs:    "test",
			Output:   []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
		},
		"cannot-find-ref-object": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "static"},
			},
			appNs: "test",
			Error: "failed to load ref object",
		},
		"modify-service": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "service"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic"},
			},
			appNs:     "test",
			IsService: true,
		},
		"by-labels": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{LabelSelector: map[string]string{"key": "value"}},
			},
			appNs: "test",
			Output: []*unstructured.Unstructured{
				createUnstructured("v1", "ConfigMap", "by-label-1", "test", map[string]interface{}{"key": "value"}),
				createUnstructured("v1", "ConfigMap", "by-label-2", "test", map[string]interface{}{"key": "value"}),
			},
		},
		"by-deprecated-labels": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{DeprecatedLabelSelector: map[string]string{"key": "value"}},
			},
			appNs: "test",
			Output: []*unstructured.Unstructured{
				createUnstructured("v1", "ConfigMap", "by-label-1", "test", map[string]interface{}{"key": "value"}),
				createUnstructured("v1", "ConfigMap", "by-label-2", "test", map[string]interface{}{"key": "value"}),
			},
		},
		"no-kind-for-resource": {
			Input: v1alpha1.ObjectReferrer{ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "unknown"}},
			appNs: "test",
			Error: "no matches",
		},
		"cross-namespace": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Namespace: "test"},
			},
			appNs:  "demo",
			Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
		},
		"cross-namespace-forbidden": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Namespace: "test"},
			},
			appNs: "demo",
			Scope: RefObjectsAvailableScopeNamespace,
			Error: "cannot refer to objects outside the application's namespace",
		},
		"cross-cluster": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Cluster: "demo"},
			},
			appNs:  "test",
			Output: []*unstructured.Unstructured{createUnstructured("v1", "ConfigMap", "dynamic", "test", nil)},
		},
		"cross-cluster-forbidden": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "configmap"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "dynamic", Cluster: "demo"},
			},
			appNs: "test",
			Scope: RefObjectsAvailableScopeCluster,
			Error: "cannot refer to objects outside control plane",
		},
		"test-cluster-scope-resource": {
			Input: v1alpha1.ObjectReferrer{
				ObjectTypeIdentifier: v1alpha1.ObjectTypeIdentifier{Resource: "clusterrole"},
				ObjectSelector:       v1alpha1.ObjectSelector{Name: "test-cluster-role"},
			},
			appNs:         "test",
			Scope:         RefObjectsAvailableScopeCluster,
			Output:        []*unstructured.Unstructured{createUnstructured("rbac.authorization.k8s.io/v1", "ClusterRole", "test-cluster-role", "", nil)},
			IsClusterRole: true,
		},
	}
	for name, tt := range testcases {
		t.Run(name, func(t *testing.T) {
			if tt.Scope == "" {
				tt.Scope = RefObjectsAvailableScopeGlobal
			}
			RefObjectsAvailableScope = tt.Scope
			output, err := SelectRefObjectsForDispatch(context.Background(), k8sClient, tt.appNs, tt.compName, tt.Input)
			if tt.Error != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.Error) {
					t.Fatalf("expected error message to contain %q, got %q", tt.Error, err.Error())
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if tt.IsService {
					if output[0].Object["kind"] != "Service" {
						t.Fatalf(`expected kind to be "Service", got %q`, output[0].Object["kind"])
					}
					if output[0].Object["spec"].(map[string]interface{})["clusterIP"] != nil {
						t.Fatalf(`expected clusterIP to be nil, got %v`, output[0].Object["spec"].(map[string]interface{})["clusterIP"])
					}
				} else {
					if tt.IsClusterRole {
						delete(output[0].Object, "rules")
					}
					if !reflect.DeepEqual(output, tt.Output) {
						t.Fatalf("expected output to be %v, got %v", tt.Output, output)
					}
				}
			}
		})
	}
}

func TestReferredObjectsDelegatingClient(t *testing.T) {
	objs := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "cm-1",
					"namespace": "ns-1",
					"labels":    map[string]interface{}{"app": "app-1"},
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "cm-2",
					"namespace": "ns-1",
					"labels":    map[string]interface{}{"app": "app-2"},
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "secret-1",
					"namespace": "ns-1",
					"labels":    map[string]interface{}{"app": "app-1"},
				},
			},
		},
	}

	delegatingClient := ReferredObjectsDelegatingClient(k8sClient, objs)

	t.Run("Get", func(t *testing.T) {
		t.Run("should get existing object", func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
			err := delegatingClient.Get(context.Background(), client.ObjectKey{Name: "cm-1", Namespace: "ns-1"}, obj)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if obj.GetName() != "cm-1" {
				t.Errorf("expected object name cm-1, got %s", obj.GetName())
			}
		})

		t.Run("should return not found for non-existing object", func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
			err := delegatingClient.Get(context.Background(), client.ObjectKey{Name: "cm-non-existent", Namespace: "ns-1"}, obj)
			if !apierrors.IsNotFound(err) {
				t.Errorf("expected not found error, got %v", err)
			}
		})

		t.Run("should return not found for different kind", func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"})
			err := delegatingClient.Get(context.Background(), client.ObjectKey{Name: "cm-1", Namespace: "ns-1"}, obj)
			if !apierrors.IsNotFound(err) {
				t.Errorf("expected not found error, got %v", err)
			}
		})
	})

	t.Run("List", func(t *testing.T) {
		t.Run("should list all objects of a kind", func(t *testing.T) {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMapList"})
			err := delegatingClient.List(context.Background(), list)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(list.Items) != 2 {
				t.Errorf("expected 2 items, got %d", len(list.Items))
			}
		})

		t.Run("should list with namespace", func(t *testing.T) {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMapList"})
			err := delegatingClient.List(context.Background(), list, client.InNamespace("ns-1"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(list.Items) != 2 {
				t.Errorf("expected 2 items, got %d", len(list.Items))
			}

			list.Items = []unstructured.Unstructured{}
			err = delegatingClient.List(context.Background(), list, client.InNamespace("ns-2"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(list.Items) != 0 {
				t.Errorf("expected 0 items, got %d", len(list.Items))
			}
		})

		t.Run("should list with label selector", func(t *testing.T) {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMapList"})
			err := delegatingClient.List(context.Background(), list, client.MatchingLabels{"app": "app-1"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(list.Items) != 1 {
				t.Errorf("expected 1 item, got %d", len(list.Items))
			}
			if list.Items[0].GetName() != "cm-1" {
				t.Errorf("expected cm-1, got %s", list.Items[0].GetName())
			}
		})
	})
}

func TestAppendUnstructuredObjects(t *testing.T) {
	testCases := map[string]struct {
		Inputs  []*unstructured.Unstructured
		Input   *unstructured.Unstructured
		Outputs []*unstructured.Unstructured
	}{
		"overlap": {
			Inputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
				"data":       "a",
			}}, {Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
				"data":       "b",
			}}},
			Input: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
				"data":       "c",
			}},
			Outputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
				"data":       "a",
			}}, {Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
				"data":       "c",
			}}},
		},
		"append": {
			Inputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
				"data":       "a",
			}}, {Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
				"data":       "b",
			}}},
			Input: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "z", "namespace": "default"},
				"data":       "c",
			}},
			Outputs: []*unstructured.Unstructured{{Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "x", "namespace": "default"},
				"data":       "a",
			}}, {Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "y", "namespace": "default"},
				"data":       "b",
			}}, {Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]interface{}{"name": "z", "namespace": "default"},
				"data":       "c",
			}}},
		},
	}
	for name, tt := range testCases {
		t.Run(name, func(t *testing.T) {
			got := AppendUnstructuredObjects(tt.Inputs, tt.Input)
			if !reflect.DeepEqual(got, tt.Outputs) {
				t.Fatalf("expected output to be %v, got %v", tt.Outputs, got)
			}
		})
	}
}

func TestConvertUnstructuredsToReferredObjects(t *testing.T) {
	uns := []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "cm-1",
				},
			},
		},
		{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name": "deploy-1",
				},
			},
		},
	}

	refObjs, err := ConvertUnstructuredsToReferredObjects(uns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refObjs) != 2 {
		t.Fatalf("expected 2 referred objects, got %d", len(refObjs))
	}

	for i, un := range uns {
		raw, err := json.Marshal(un)
		if err != nil {
			t.Fatalf("failed to marshal unstructured: %v", err)
		}
		expected := common.ReferredObject{
			RawExtension: runtime.RawExtension{Raw: raw},
		}
		if !reflect.DeepEqual(refObjs[i], expected) {
			t.Errorf("expected refObj %v, got %v", expected, refObjs[i])
		}
	}
}
