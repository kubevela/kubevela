/*
Copyright 2022 The KubeVela Authors.

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

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	appv1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
)

func TestToPercentageStr(t *testing.T) {
	t.Run("valid percentage", func(t *testing.T) {
		var v1, v2 int64 = 10, 100
		assert.Equal(t, "10%", ToPercentageStr(v1, v2))
	})
	t.Run("division by zero", func(t *testing.T) {
		var v1, v2 int64 = 10, 0
		assert.Equal(t, "N/A", ToPercentageStr(v1, v2))
	})
}

func TestToPercentage(t *testing.T) {
	t.Run("valid percentage", func(t *testing.T) {
		var v1, v2 int64 = 10, 100
		assert.Equal(t, 10, ToPercentage(v1, v2))
	})
	t.Run("division by zero", func(t *testing.T) {
		var v1, v2 int64 = 10, 0
		assert.Equal(t, 0, ToPercentage(v1, v2))
	})
	t.Run("floor", func(t *testing.T) {
		var v1, v2 int64 = 1, 3
		assert.Equal(t, 33, ToPercentage(v1, v2))
	})
}

func TestGetPodResourceSpecAndUsage(t *testing.T) {
	s := runtime.NewScheme()
	v1.AddToScheme(s)
	k8sClient := fake.NewClientBuilder().WithScheme(s).Build()

	quantityLimitsCPU, _ := resource.ParseQuantity("10m")
	quantityLimitsMemory, _ := resource.ParseQuantity("10Mi")
	quantityRequestsCPU, _ := resource.ParseQuantity("100m")
	quantityRequestsMemory, _ := resource.ParseQuantity("50Mi")
	quantityUsageCPU, _ := resource.ParseQuantity("8m")
	quantityUsageMemory, _ := resource.ParseQuantity("20Mi")

	pod := &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Requests: map[v1.ResourceName]resource.Quantity{"memory": quantityRequestsMemory, "cpu": quantityRequestsCPU},
						Limits:   map[v1.ResourceName]resource.Quantity{"memory": quantityLimitsMemory, "cpu": quantityLimitsCPU},
					},
				},
			},
		},
	}
	podMetric := &v1beta1.PodMetrics{
		Containers: []v1beta1.ContainerMetrics{
			{
				Name: "",
				Usage: map[v1.ResourceName]resource.Quantity{
					"memory": quantityUsageMemory, "cpu": quantityUsageCPU,
				},
			},
		},
	}

	spec, usage := GetPodResourceSpecAndUsage(k8sClient, pod, podMetric)
	assert.Equal(t, int64(8), usage.CPU)
	assert.Equal(t, int64(20971520), usage.Mem)
	assert.Equal(t, int64(10), spec.Lcpu)
	assert.Equal(t, int64(10485760), spec.Lmem)
	assert.Equal(t, int64(100), spec.Rcpu)
	assert.Equal(t, int64(52428800), spec.Rmem)
}

func TestGetPodStorage(t *testing.T) {
	s := runtime.NewScheme()
	v1.AddToScheme(s)

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc",
			Namespace: "default",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(pvc).Build()

	podWithPVC := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "test-storage",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "test-pvc",
						},
					},
				},
			},
		},
	}

	podWithoutPVC := &v1.Pod{
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "test-emptydir",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	podWithNonExistentPVC := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
		Spec: v1.PodSpec{
			Volumes: []v1.Volume{
				{
					Name: "test-storage",
					VolumeSource: v1.VolumeSource{
						PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
							ClaimName: "non-existent-pvc",
						},
					},
				},
			},
		},
	}

	t.Run("pod with pvc", func(t *testing.T) {
		storages := GetPodStorage(k8sClient, podWithPVC)
		assert.Equal(t, 1, len(storages))
		assert.Equal(t, "test-pvc", storages[0].Name)
	})

	t.Run("pod without pvc", func(t *testing.T) {
		storages := GetPodStorage(k8sClient, podWithoutPVC)
		assert.Equal(t, 0, len(storages))
	})

	t.Run("pod with non-existent pvc", func(t *testing.T) {
		storages := GetPodStorage(k8sClient, podWithNonExistentPVC)
		assert.Equal(t, 0, len(storages))
	})
}

func TestGetPodOfManagedResource(t *testing.T) {
	s := runtime.NewScheme()
	v1.AddToScheme(s)
	appv1beta1.AddToScheme(s)

	app := &appv1beta1.Application{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
	podUnstructured, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	assert.NoError(t, err)

	rt := &appv1beta1.ResourceTracker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app-v1", // A tracker name
			Namespace: "default",
			Labels: map[string]string{
				"app.oam.dev/name":      "test-app",
				"app.oam.dev/namespace": "default",
			},
		},
		Spec: appv1beta1.ResourceTrackerSpec{
			Type: appv1beta1.ResourceTrackerTypeRoot,
			ManagedResources: []appv1beta1.ManagedResource{
				{
					ClusterObjectReference: common.ClusterObjectReference{
						ObjectReference: v1.ObjectReference{
							Kind:       "Pod",
							APIVersion: "v1",
							Name:       "test-pod",
							Namespace:  "default",
						},
					},
					OAMObjectReference: common.OAMObjectReference{
						Component: "test-component",
					},
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(s).WithObjects(app, rt, &unstructured.Unstructured{Object: podUnstructured}).Build()

	t.Run("get existing pod", func(t *testing.T) {
		pods := GetPodOfManagedResource(k8sClient, app, "test-component")
		assert.Equal(t, 2, len(pods))
		assert.Equal(t, "test-pod", pods[0].Name)
		assert.Equal(t, "test-pod", pods[1].Name)
	})

	t.Run("get pod for non-existent component", func(t *testing.T) {
		pods := GetPodOfManagedResource(k8sClient, app, "non-existent-component")
		assert.Equal(t, 0, len(pods))
	})
}
