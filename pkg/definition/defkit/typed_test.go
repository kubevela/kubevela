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

package defkit_test

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/oam-dev/kubevela/pkg/definition/defkit"
)

func TestFromTyped_Deployment(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(3)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
		},
	}

	r, err := defkit.FromTyped(deployment)
	if err != nil {
		t.Fatalf("FromTyped failed: %v", err)
	}

	// Verify basic properties
	if r.APIVersion() != "apps/v1" {
		t.Errorf("Expected apiVersion apps/v1, got %s", r.APIVersion())
	}
	if r.Kind() != "Deployment" {
		t.Errorf("Expected kind Deployment, got %s", r.Kind())
	}

	// Verify operations were created
	ops := r.Ops()
	if len(ops) == 0 {
		t.Error("Expected Set operations to be created")
	}
}

func TestFromTyped_ConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-config",
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	r, err := defkit.FromTyped(cm)
	if err != nil {
		t.Fatalf("FromTyped failed: %v", err)
	}

	// Verify basic properties
	if r.APIVersion() != "v1" {
		t.Errorf("Expected apiVersion v1, got %s", r.APIVersion())
	}
	if r.Kind() != "ConfigMap" {
		t.Errorf("Expected kind ConfigMap, got %s", r.Kind())
	}
}

func TestFromTyped_MissingTypeMeta(t *testing.T) {
	// Create object without TypeMeta set
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
		},
	}

	_, err := defkit.FromTyped(deployment)
	if err == nil {
		t.Error("Expected error when TypeMeta is not set")
	}
}

func TestMustFromTyped_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustFromTyped to panic when TypeMeta is not set")
		}
	}()

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
		},
	}

	defkit.MustFromTyped(deployment)
}

func TestMustFromTyped_Success(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
		},
	}

	r := defkit.MustFromTyped(deployment)
	if r == nil {
		t.Error("Expected non-nil Resource")
	}
	if r.Kind() != "Deployment" {
		t.Errorf("Expected kind Deployment, got %s", r.Kind())
	}
}

func TestFromTyped_ChainAdditionalOperations(t *testing.T) {
	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-app",
		},
	}

	r, err := defkit.FromTyped(deployment)
	if err != nil {
		t.Fatalf("FromTyped failed: %v", err)
	}

	// Chain additional operations
	image := defkit.String("image").Required()
	r.Set("spec.template.spec.containers[0].image", image)

	ops := r.Ops()
	foundImageOp := false
	for _, op := range ops {
		if setOp, ok := op.(*defkit.SetOp); ok {
			if setOp.Path() == "spec.template.spec.containers[0].image" {
				foundImageOp = true
				break
			}
		}
	}
	if !foundImageOp {
		t.Error("Expected to find chained Set operation for image")
	}
}
