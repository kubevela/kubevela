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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"

	"github.com/oam-dev/kubevela/pkg/oam/mock"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var (
	errNotFound = errors.New("HealthScope not found")
	// errGetResources = errors.New("cannot get resources")
)

func TestHealthScope(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HealthScope Suite")
}

var _ = Describe("HealthScope Controller Reconcile Test", func() {
	mockMgr := &mock.Manager{
		Client: &test.MockClient{},
	}
	MockHealthyChecker := WorkloadHealthCheckFn(
		func(context.Context, client.Client, corev1.ObjectReference, string) *WorkloadHealthCondition {
			return &WorkloadHealthCondition{HealthStatus: StatusHealthy}
		})
	MockUnhealthyChecker := WorkloadHealthCheckFn(
		func(context.Context, client.Client, corev1.ObjectReference, string) *WorkloadHealthCondition {
			return &WorkloadHealthCondition{HealthStatus: StatusUnhealthy}
		})
	reconciler := NewReconciler(mockMgr,
		WithRecorder(event.NewNopRecorder()),
		WithChecker(MockHealthyChecker),
	)

	hs := v1alpha2.HealthScope{Spec: v1alpha2.HealthScopeSpec{WorkloadReferences: []corev1.ObjectReference{
		// add one wlRef to trigger mockChecker
		{
			APIVersion: "mock",
			Kind:       "mock",
		},
	}}}

	BeforeEach(func() {
		logf.Log.Info("Set up resources before an unit test")
		// remove built-in checkers then fulfill mock checkers
		reconciler.checkers = []WorloadHealthChecker{}
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after an unit test")
	})

	It("Test HealthScope Not Found", func() {
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context,
				key client.ObjectKey, obj runtime.Object) error {
				return errNotFound
			},
		}
		result, err := reconciler.Reconcile(reconcile.Request{})
		Expect(result).Should(Equal(reconcile.Result{}))
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errNotFound, errGetHealthScope)))
	})

	It("Test Reconcile UpdateHealthStatus Error", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return errMockErr
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errMockErr, errUpdateHealthScopeStatus)))
	})

	It("Test Reconcile Success with healthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(BeNil())
	})

	It("Test Reconcile Success with unhealthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockUnhealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(reconcile.Request{})
		Expect(err).Should(BeNil())
	})
})

var _ = Describe("Test GetScopeHealthStatus", func() {
	ctx := context.Background()
	mockMgr := &mock.Manager{
		Client: &test.MockClient{},
	}
	reconciler := NewReconciler(mockMgr,
		WithRecorder(event.NewNopRecorder()),
	)
	reconciler.client = test.NewMockClient()

	hs := v1alpha2.HealthScope{}

	var cwRef, deployRef, svcRef corev1.ObjectReference
	cwRef.SetGroupVersionKind(v1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	cwRef.Name = "cw"
	deployRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindDeployment))
	deployRef.Name = "deploy"
	svcRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindService))

	cw := v1alpha2.ContainerizedWorkload{}
	cw.SetGroupVersionKind(v1alpha2.SchemeGroupVersion.WithKind(kindContainerizedWorkload))
	cw.Status.Resources = []corev1.ObjectReference{deployRef, svcRef}

	hDeploy := appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Replicas: &varInt1,
		},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1, // healthy
		},
	}
	hDeploy.SetName("deploy")
	hDeploy.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindDeployment))

	uhGeneralRef := corev1.ObjectReference{
		APIVersion: "unknown",
		Kind:       "unknown",
		Name:       "unhealthyGeneral",
	}
	uhDeploy := hDeploy
	uhDeploy.Status.ReadyReplicas = 0 // unhealthy
	uhGeneralWL := &unstructured.Unstructured{Object: make(map[string]interface{})}
	fieldpath.Pave(uhGeneralWL.Object).SetValue("status.readyReplicas", 0)           // healthy
	fieldpath.Pave(uhGeneralWL.Object).SetValue("metadata.name", "unhealthyGeneral") // healthy
	unsupporttedWL := &unstructured.Unstructured{Object: make(map[string]interface{})}
	fieldpath.Pave(unsupporttedWL.Object).SetValue("status.unknown", 1) // healthy

	BeforeEach(func() {
		logf.Log.Info("Set up resources before an unit test")
		hs.Spec.WorkloadReferences = []corev1.ObjectReference{}
	})

	AfterEach(func() {
		logf.Log.Info("Clean up resources after an unit test")
	})

	// use ContainerizedWorkload and Deployment checker
	It("Test healthy scope", func() {
		tests := []struct {
			caseName           string
			hsWorkloadRefs     []corev1.ObjectReference
			mockGetFn          test.MockGetFn
			wantScopeCondition ScopeHealthCondition
		}{
			{
				caseName:       "2 supportted workloads(cw,deploy)",
				hsWorkloadRefs: []corev1.ObjectReference{cwRef, deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if o, ok := obj.(*v1alpha2.ContainerizedWorkload); ok {
						*o = cw
					}
					if o, ok := obj.(*appsv1.Deployment); ok {
						*o = hDeploy
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusHealthy,
					Total:              int64(2),
					HealthyWorkloads:   int64(2),
					UnhealthyWorkloads: 0,
					UnknownWorkloads:   0,
				},
			},
		}
		for _, tc := range tests {
			By("Running: " + tc.caseName)
			mockClient := &test.MockClient{
				MockGet: tc.mockGetFn,
			}
			reconciler.client = mockClient
			hs.Spec.WorkloadReferences = tc.hsWorkloadRefs
			result, _ := reconciler.GetScopeHealthStatus(ctx, &hs)
			Expect(result).ShouldNot(BeNil())
			Expect(result).Should(Equal(tc.wantScopeCondition))
		}
	})

	// use ContainerizedWorkload and Deployment checker
	It("Test unhealthy scope", func() {
		tests := []struct {
			caseName           string
			hsWorkloadRefs     []corev1.ObjectReference
			mockGetFn          test.MockGetFn
			wantScopeCondition ScopeHealthCondition
		}{
			{
				caseName:       "2 supportted workloads but one is unhealthy",
				hsWorkloadRefs: []corev1.ObjectReference{cwRef, deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					switch o := obj.(type) {
					case *v1alpha2.ContainerizedWorkload:
						*o = cw
					case *appsv1.Deployment:
						*o = hDeploy
					case *unstructured.Unstructured:
						// return err when get svc of cw, then check fails
						if key.Name == "cw" || key.Name == "deploy" {
							return nil
						}
						return errMockErr
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusUnhealthy,
					Total:              int64(2),
					HealthyWorkloads:   int64(1),
					UnhealthyWorkloads: int64(1),
					UnknownWorkloads:   0,
				},
			},
			{
				caseName:       "1 healthy supportted workload and 1 unsupportted workloads",
				hsWorkloadRefs: []corev1.ObjectReference{cwRef, uhGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					switch o := obj.(type) {
					case *v1alpha2.ContainerizedWorkload:
						*o = cw
					case *appsv1.Deployment:
						*o = hDeploy
					case *unstructured.Unstructured:
						*o = *unsupporttedWL
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusUnhealthy,
					Total:              int64(2),
					HealthyWorkloads:   int64(1),
					UnhealthyWorkloads: 0,
					UnknownWorkloads:   int64(1),
				},
			},
		}

		for _, tc := range tests {
			By("Running: " + tc.caseName)
			mockClient := &test.MockClient{
				MockGet: tc.mockGetFn,
			}
			reconciler.client = mockClient
			hs.Spec.WorkloadReferences = tc.hsWorkloadRefs
			result, _ := reconciler.GetScopeHealthStatus(ctx, &hs)
			Expect(result).ShouldNot(BeNil())
			Expect(result).Should(Equal(tc.wantScopeCondition))
		}
	})
})
