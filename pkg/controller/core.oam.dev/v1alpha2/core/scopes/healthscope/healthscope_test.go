/*
Copyright 2021 The Crossplane Authors.

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
	"fmt"
	"sort"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

const (
	namespace = "ns"
)

var (
	ctx        = context.Background()
	errMockErr = errors.New("get error")
	varInt1    = int32(1)
)


func TestCheckDeploymentHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	deployRef := corev1.ObjectReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deployRef.Name = "deploy"

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
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "deploy" {
						deployObj, err := util.Object2Unstructured(apps.Deployment{
							Spec: apps.DeploymentSpec{
								Replicas: &varInt1,
							},
							Status: apps.DeploymentStatus{
								ReadyReplicas: 1, // healthy
							},
						})
						if err != nil {
							return err
						}
						*o = *deployObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not ready",
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "deploy" {
						deployObj, err := util.Object2Unstructured(apps.Deployment{
							Spec: apps.DeploymentSpec{
								Replicas: &varInt1,
							},
							Status: apps.DeploymentStatus{
								ReadyReplicas: 0, // unhealthy
							},
						})
						if err != nil {
							return err
						}
						*o = *deployObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for deployment not found",
			wlRef:    deployRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				return errMockErr
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckDeploymentHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestCheckStatefulsetHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	stsRef := corev1.ObjectReference{}
	stsRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindStatefulSet))
	stsRef.Name = "sts"

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
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "sts" {
						stsObj, err := util.Object2Unstructured(apps.StatefulSet{
							Spec: apps.StatefulSetSpec{
								Replicas: &varInt1,
							},
							Status: apps.StatefulSetStatus{
								ReadyReplicas: 1, // healthy
							},
						})
						if err != nil {
							return err
						}
						*o = *stsObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for statefulset not ready",
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "sts" {
						stsObj, err := util.Object2Unstructured(apps.StatefulSet{
							Spec: apps.StatefulSetSpec{
								Replicas: &varInt1,
							},
							Status: apps.StatefulSetStatus{
								ReadyReplicas: 0, // unhealthy
							},
						})
						if err != nil {
							return err
						}
						*o = *stsObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for statefulset not found",
			wlRef:    stsRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				return errMockErr
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckStatefulsetHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestCheckDaemonsetHealth(t *testing.T) {
	mockClient := test.NewMockClient()
	dstRef := corev1.ObjectReference{}
	dstRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDaemonSet))
	dstRef.Name = "dst"

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
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "dst" {
						dstObj, err := util.Object2Unstructured(apps.DaemonSet{
							Status: apps.DaemonSetStatus{
								NumberUnavailable: 0, // healthy
							},
						})
						if err != nil {
							return err
						}
						*o = *dstObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "unhealthy for daemonset not ready",
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "dst" {
						dstObj, err := util.Object2Unstructured(apps.DaemonSet{
							Status: apps.DaemonSetStatus{
								NumberUnavailable: 1, // unhealthy
							},
						})
						if err != nil {
							return err
						}
						*o = *dstObj
					}
					return nil
				}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "unhealthy for daemonset not found",
			wlRef:    dstRef,
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				return errMockErr
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckDaemonsetHealth(ctx, mockClient, tc.wlRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestCheckUnknownWorkload(t *testing.T) {
	mockError := errors.New("mock error")
	mockClient := test.NewMockClient()
	unknownWL := corev1.ObjectReference{}
	tests := []struct {
		caseName  string
		mockGetFn test.MockGetFn
		expect    *WorkloadHealthCondition
	}{
		{
			caseName: "cannot get workload",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				return mockError
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnknown,
				Diagnosis:    errors.Wrap(mockError, errHealthCheck).Error(),
			},
		},
		{
			caseName: "unknown workload with status",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				o, _ := obj.(*unstructured.Unstructured)
				*o = unstructured.Unstructured{}
				o.Object = make(map[string]interface{})
				fieldpath.Pave(o.Object).SetValue("status.unknown", 1)
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus:   StatusUnknown,
				Diagnosis:      fmt.Sprintf(infoFmtUnknownWorkload, "", ""),
				WorkloadStatus: "{\"unknown\":1}",
			},
		},
		{
			caseName: "unknown workload without status",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				o, _ := obj.(*unstructured.Unstructured)
				*o = unstructured.Unstructured{}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus:   StatusUnknown,
				Diagnosis:      fmt.Sprintf(infoFmtUnknownWorkload, "", ""),
				WorkloadStatus: "null",
			},
		},
	}
	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			result := CheckUnknownWorkload(ctx, mockClient, unknownWL, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect, result, tc.caseName)
			}
		}(t)
	}
}

func TestCheckVersionEnabledComponent(t *testing.T) {
	deployRef := corev1.ObjectReference{}
	deployRef.SetGroupVersionKind(apps.SchemeGroupVersion.WithKind(kindDeployment))
	deployRef.Name = "main-workload"
	deployObj := apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: "main-workload",
		},
		Spec: apps.DeploymentSpec{
			Replicas: &varInt1,
		},
		Status: apps.DeploymentStatus{
			ReadyReplicas: 1, // healthy
		}}

	peerDeployObj := apps.Deployment{
		ObjectMeta: v1.ObjectMeta{
			Name: "peer-workload",
		},
		Spec: apps.DeploymentSpec{
			Replicas: &varInt1,
		},
		Status: apps.DeploymentStatus{
			ReadyReplicas: 1, // healthy
		}}

	mockClient := test.NewMockClient()
	tests := []struct {
		caseName   string
		mockGetFn  test.MockGetFn
		mockListFn test.MockListFn
		expect     *WorkloadHealthCondition
	}{
		{
			caseName: "peer workload is healthy",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "main-workload" {
						deploy, err := util.Object2Unstructured(deployObj)
						if err != nil {
							return err
						}
						*o = *deploy
					} else if key.Name == "peer-workload" {
						peerDeploy, err := util.Object2Unstructured(peerDeployObj)
						if err != nil {
							return err
						}
						*o = *peerDeploy
					} else {
						o.SetLabels(map[string]string{
							oam.LabelAppComponent: "test-comp",
							oam.LabelAppName:      "test-app",
						})
					}
					return nil
				}
				return nil
			},
			mockListFn: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				l, _ := list.(*unstructured.UnstructuredList)
				u := unstructured.Unstructured{}
				u.SetAPIVersion("apps/v1")
				u.SetKind("Deployment")
				u.SetName("peer-workload")
				l.Items = []unstructured.Unstructured{u}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusHealthy,
			},
		},
		{
			caseName: "peer workload is unhealthy",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "main-workload" {
						deploy, err := util.Object2Unstructured(deployObj)
						if err != nil {
							return err
						}
						*o = *deploy
						o.SetLabels(map[string]string{
							oam.LabelAppComponent: "test-comp",
							oam.LabelAppName:      "test-app",
						})
					} else if key.Name == "peer-workload" {
						peerDeployCopy := peerDeployObj.DeepCopy()
						peerDeployCopy.Status.ReadyReplicas = int32(0)
						peerDeploy, err := util.Object2Unstructured(peerDeployCopy)
						if err != nil {
							return err
						}
						*o = *peerDeploy
					}
					return nil
				}
				return nil
			},
			mockListFn: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				l, _ := list.(*unstructured.UnstructuredList)
				u := unstructured.Unstructured{}
				u.SetAPIVersion("apps/v1")
				u.SetKind("Deployment")
				u.SetName("peer-workload")
				l.Items = []unstructured.Unstructured{u}
				return nil
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
		{
			caseName: "error occurs when get peer workload",
			mockGetFn: func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				switch o := obj.(type) {
				case *unstructured.Unstructured:
					if key.Name == "main-workload" {
						deploy, err := util.Object2Unstructured(deployObj)
						if err != nil {
							return err
						}
						*o = *deploy
						o.SetLabels(map[string]string{
							oam.LabelAppComponent: "test-comp",
							oam.LabelAppName:      "test-app",
						})
					}
					return nil
				}
				return nil
			},
			mockListFn: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return errMockErr
			},
			expect: &WorkloadHealthCondition{
				HealthStatus: StatusUnhealthy,
			},
		},
	}

	for _, tc := range tests {
		func(t *testing.T) {
			mockClient.MockGet = tc.mockGetFn
			mockClient.MockList = tc.mockListFn
			checker := WorkloadHealthCheckFn(CheckDeploymentHealth)
			result := checker.Check(ctx, mockClient, deployRef, namespace)
			if tc.expect == nil {
				assert.Nil(t, result, tc.caseName)
			} else {
				assert.Equal(t, tc.expect.HealthStatus, result.HealthStatus, tc.caseName)
			}
		}(t)
	}
}

func TestPeerHealthConditionsSort(t *testing.T) {
	tests := []struct {
		caseName string
		d        []string
		w        []string
	}{
		{
			caseName: "all has qualified revision name",
			d:        []string{"comp-v1", "comp-v2", "comp-v12"},
			w:        []string{"comp-v12", "comp-v2", "comp-v1"},
		},
		{
			caseName: "part has qualified revision name",
			d:        []string{"comp-v1", "comp", "comp-v2", "comp-v12"},
			w:        []string{"comp-v12", "comp-v2", "comp-v1", "comp"},
		},
	}
	for _, tc := range tests {
		func(t *testing.T) {
			data := make(PeerHealthConditions, len(tc.d))
			want := make(PeerHealthConditions, len(tc.w))
			for i, v := range tc.d {
				data[i] = WorkloadHealthCondition{
					TargetWorkload: corev1.ObjectReference{Name: v},
				}
			}
			for i, v := range tc.w {
				want[i] = WorkloadHealthCondition{
					TargetWorkload: corev1.ObjectReference{Name: v},
				}
			}
			sort.Sort(data)
			if diff := cmp.Diff(data, want); diff != "" {
				t.Errorf("didn't get expected sorted result %s", diff)
			}
		}(t)
	}

}
