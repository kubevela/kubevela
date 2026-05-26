/*
Copyright 2024 The KubeVela Authors.

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
package oam

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterFunctions(t *testing.T) {
	obj := &corev1.ConfigMap{}

	SetCluster(obj, "cluster-1")
	if GetCluster(obj) != "cluster-1" {
		t.Fatalf("expected cluster-1, got %s", GetCluster(obj))
	}

	SetClusterIfEmpty(obj, "cluster-2")
	if GetCluster(obj) != "cluster-1" {
		t.Fatalf("should not overwrite existing cluster")
	}

	obj2 := &corev1.ConfigMap{}
	SetClusterIfEmpty(obj2, "cluster-2")
	if GetCluster(obj2) != "cluster-2" {
		t.Fatalf("expected cluster-2, got %s", GetCluster(obj2))
	}
}

func TestPublishVersion(t *testing.T) {
	obj := &corev1.ConfigMap{}
	SetPublishVersion(obj, "v1")
	if GetPublishVersion(obj) != "v1" {
		t.Fatalf("expected v1, got %s", GetPublishVersion(obj))
	}
}

func TestControllerRequirement(t *testing.T) {
	obj := &corev1.ConfigMap{}
	SetControllerRequirement(obj, "req1")
	if GetControllerRequirement(obj) != "req1" {
		t.Fatalf("expected req1, got %s", GetControllerRequirement(obj))
	}
	SetControllerRequirement(obj, "")
	if GetControllerRequirement(obj) != "" {
		t.Fatalf("expected empty after delete")
	}
}

func TestGetLastAppliedTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	obj := &corev1.ConfigMap{}
	obj.SetAnnotations(map[string]string{
		AnnotationLastAppliedTime: now.Format(time.RFC3339),
	})
	got := GetLastAppliedTime(obj)
	if !got.Equal(now) {
		t.Fatalf("expected %v, got %v", now, got)
	}

	obj2 := &corev1.ConfigMap{}
	obj2.SetCreationTimestamp(metav1.NewTime(now))
	got2 := GetLastAppliedTime(obj2)
	if !got2.Equal(now) {
		t.Fatalf("expected fallback to creation time, got %v", got2)
	}
}
