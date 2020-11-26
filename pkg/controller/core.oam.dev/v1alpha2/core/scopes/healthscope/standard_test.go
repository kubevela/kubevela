package healthscope

import (
	"context"
	"testing"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

func TestCheckPodSpecWorkloadHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	scRef := runtimev1alpha1.TypedReference{}
	scRef.SetGroupVersionKind(podSpecWorkloadGVK)

	deployRef := runtimev1alpha1.TypedReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	svcRef := runtimev1alpha1.TypedReference{}
	svcRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindService))

	deployRefData, _ := util.Object2Map(deployRef)
	svcRefData, _ := util.Object2Map(svcRef)

	scUnstructured := unstructured.Unstructured{}
	scUnstructured.SetGroupVersionKind(podSpecWorkloadGVK)
	unstructured.SetNestedSlice(scUnstructured.Object, []interface{}{deployRefData, svcRefData}, "status", "resources")

	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		wlRef     runtimev1alpha1.TypedReference
		expect    *WorkloadHealthCondition
	}{
		{
			caseName: "not matched checker",
			wlRef:    runtimev1alpha1.TypedReference{},
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
