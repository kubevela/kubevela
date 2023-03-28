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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"

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
				key client.ObjectKey, obj client.Object) error {
				return errNotFound
			},
		}
		result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{})
		Expect(result).Should(Equal(reconcile.Result{}))
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errNotFound, errGetHealthScope)))
	})

	It("Test Reconcile UpdateHealthStatus Error", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				if o, ok := obj.(*v1beta1.Application); ok {
					*o = v1beta1.Application{}
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return errMockErr
			},
			MockStatusPatch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{})
		Expect(err).Should(util.BeEquivalentToError(errors.Wrap(errMockErr, errUpdateHealthScopeStatus)))
	})

	It("Test Reconcile Success with healthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockHealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				if o, ok := obj.(*v1beta1.Application); ok {
					*o = v1beta1.Application{}
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return nil
			},
			MockStatusPatch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{})
		Expect(err).Should(BeNil())
	})

	It("Test Reconcile Success with unhealthy scope", func() {
		reconciler.checkers = append(reconciler.checkers, MockUnhealthyChecker)
		reconciler.client = &test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if o, ok := obj.(*v1alpha2.HealthScope); ok {
					*o = hs
				}
				if o, ok := obj.(*v1beta1.Application); ok {
					*o = v1beta1.Application{}
				}
				return nil
			},
			MockStatusUpdate: func(_ context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return nil
			},
			MockStatusPatch: func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				return nil
			},
		}
		_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{})
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

	var deployRef, svcRef corev1.ObjectReference
	deployRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindDeployment))
	deployRef.Name = "deploy"
	svcRef.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind(kindService))

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

	// use Deployment checker
	It("Test healthy scope", func() {
		tests := []struct {
			caseName           string
			hsWorkloadRefs     []corev1.ObjectReference
			mockGetFn          test.MockGetFn
			wantScopeCondition ScopeHealthCondition
		}{
			{
				caseName:       "1 supportted workload(deploy)",
				hsWorkloadRefs: []corev1.ObjectReference{deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					switch o := obj.(type) {
					case *unstructured.Unstructured:
						if key.Name == "deploy" {
							deployObj, err := util.Object2Unstructured(hDeploy)
							if err != nil {
								return err
							}
							*o = *deployObj
						}
						return nil
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusHealthy,
					Total:              int64(1),
					HealthyWorkloads:   int64(1),
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

	// use Deployment checker
	It("Test unhealthy scope", func() {
		tests := []struct {
			caseName           string
			hsWorkloadRefs     []corev1.ObjectReference
			mockGetFn          test.MockGetFn
			wantScopeCondition ScopeHealthCondition
		}{
			{
				caseName:       "1 unhealthy workload",
				hsWorkloadRefs: []corev1.ObjectReference{deployRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					switch o := obj.(type) {
					case *unstructured.Unstructured:
						// return err when get svc of cw, then check fails
						if key.Name == "deploy" {
							deployObj, err := util.Object2Unstructured(uhDeploy)
							if err != nil {
								return err
							}
							*o = *deployObj
							return nil
						}
						return errMockErr
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusUnhealthy,
					Total:              int64(1),
					HealthyWorkloads:   0,
					UnhealthyWorkloads: int64(1),
					UnknownWorkloads:   0,
				},
			},
			{
				caseName:       "1 unsupportted workloads",
				hsWorkloadRefs: []corev1.ObjectReference{uhGeneralRef},
				mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
					switch o := obj.(type) {
					case *unstructured.Unstructured:
						if key.Name == "deploy" {
							deployObj, err := util.Object2Unstructured(hDeploy)
							if err != nil {
								return err
							}
							*o = *deployObj
							return nil
						}
						*o = *unsupporttedWL
					}
					return nil
				},
				wantScopeCondition: ScopeHealthCondition{
					HealthStatus:       StatusUnhealthy,
					Total:              int64(1),
					HealthyWorkloads:   0,
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
