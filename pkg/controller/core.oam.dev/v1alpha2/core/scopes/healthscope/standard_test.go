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

package healthscope

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestCheckPodSpecWorkloadHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	scRef := corev1.ObjectReference{}
	scRef.SetGroupVersionKind(podSpecWorkloadGVK)

	deployRef := corev1.ObjectReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	svcRef := corev1.ObjectReference{}
	svcRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindService))

	deployRefData, _ := util.Object2Map(deployRef)
	svcRefData, _ := util.Object2Map(svcRef)

	scUnstructured := unstructured.Unstructured{}
	scUnstructured.SetGroupVersionKind(podSpecWorkloadGVK)
	unstructured.SetNestedSlice(scUnstructured.Object, []interface{}{deployRefData, svcRefData}, "status", "resources")

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     corev1.ObjectReference
		expect    *WorkloadHealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    corev1.ObjectReference{},
			expect:   nil,
		},
		{
			caseName: "healthy workload",
			wlRef:    scRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*unstructured.Unstructured); ok {
					*o = scUnstructured
					return nil
				}
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Spec: apps.DeploymentSpec{
							Replicas: &varInt1,
						},
						Status: apps.DeploymentStatus{
							ReadyReplicas: 1, // healthy
						},
					}
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not ready",
			wlRef:    scRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*unstructured.Unstructured); ok {
					*o = scUnstructured
					return nil
				}
				if o, ok := obj.(*apps.Deployment); ok {
					*o = apps.Deployment{
						Spec: apps.DeploymentSpec{
							Replicas: &varInt1,
						},
						Status: apps.DeploymentStatus{
							ReadyReplicas: 0, // unhealthy
						},
					}
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for PodSpecWorkload not found",
			wlRef:    scRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				return errMockErr
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not found",
			wlRef:    scRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				if o, ok := obj.(*unstructured.Unstructured); ok {
					*o = scUnstructured
					return nil
				}
				if _, ok := obj.(*apps.Deployment); ok {
					return errMockErr
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckPodSpecWorkloadHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}

		}(t)
	}
}
