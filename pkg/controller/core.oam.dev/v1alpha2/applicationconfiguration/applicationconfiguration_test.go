/*
Copyright 2019 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	htcp://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package applicationconfiguration

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam/mock"
	"github.com/oam-dev/kubevela/pkg/utils/apply"
)

// OAMApplicationReconciler implements controller runtime Reconciler interface
var _ reconcile.Reconciler = &OAMApplicationReconciler{}

type acParam func(*v1alpha2.ApplicationConfiguration)

func withConditions(c ...condition.Condition) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.SetConditions(c...)
	}
}

func withWorkloadStatuses(ws ...v1alpha2.WorkloadStatus) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.Status.Workloads = ws
	}
}

func withDependencyStatus(s v1alpha2.DependencyStatus) acParam {
	return func(ac *v1alpha2.ApplicationConfiguration) {
		ac.Status.Dependency = s
	}
}

func ac(p ...acParam) *v1alpha2.ApplicationConfiguration {
	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{workloadScopeFinalizer},
		},
	}
	for _, fn := range p {
		fn(ac)
	}
	return ac
}

func TestReconciler(t *testing.T) {
	errBoom := errors.New("boom")
	errUnexpectedStatus := errors.New("unexpected status")

	namespace := "ns"
	componentName := "coolcomponent"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	now := metav1.Now()

	depStatus := v1alpha2.DependencyStatus{
		Unsatisfied: []v1alpha2.UnstaifiedDependency{{
			From: v1alpha2.DependencyFromObject{
				ObjectReference: corev1.ObjectReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				FieldPath: "status.key",
			},
			To: v1alpha2.DependencyToObject{
				ObjectReference: corev1.ObjectReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				FieldPaths: []string{"spec.key"},
			},
		}},
	}

	mockGetAppConfigFn := func(_ context.Context, key client.ObjectKey, obj client.Object) error {
		if o, ok := obj.(*v1alpha2.ApplicationConfiguration); ok {
			*o = v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Finalizers: []string{workloadScopeFinalizer},
				},
			}
		}
		return nil
	}

	type args struct {
		m manager.Manager
		o []ReconcilerOption
	}
	type want struct {
		result reconcile.Result
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetApplicationConfigurationError": {
			reason: "Errors getting the ApplicationConfiguration under reconciliation should be returned",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(errBoom),
					},
				},
			},
			want: want{
				result: reconcile.Result{},
				err:    errors.Wrap(errBoom, errGetAppConfig),
			},
		},
		"RenderComponentsError": {
			reason: "Errors rendering components should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: mockGetAppConfigFn,
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(withConditions(condition.ReconcileError(errors.Wrap(errBoom, errRenderComponents))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}

							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return nil, &v1alpha2.DependencyStatus{}, errBoom
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"ApplyComponentsError": {
			reason: "Errors applying components should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: mockGetAppConfigFn,
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(withConditions(condition.ReconcileError(errors.Wrap(errBoom, errApplyComponents))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return errBoom
					}}),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"GCDeleteError": {
			reason: "Errors deleting a garbage collected resource should be reflected as a status condition",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(errBoom),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(withConditions(condition.ReconcileError(errors.Wrap(errBoom, errGCComponent))))
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration)); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*workload}
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"Has dependency": {
			reason: "dependency should be reflected in status and wait time should align",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: corev1.ObjectReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
								withDependencyStatus(depStatus),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: corev1.ObjectReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
								withDependencyStatus(depStatus),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &depStatus, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: 10 * time.Second},
			},
		},
		"FailedPreHook": {
			reason: "Rendered workloads should be reflected in status",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileError(errors.Wrap(errBoom, errExecutePrehooks))),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
					WithPrehook("preHookSuccess", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
					})),
					WithPrehook("preHookFailed", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, errBoom
					})),
					WithPosthook("postHook", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{}, nil
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"FailedPostHook": {
			reason: "Rendered workloads should be reflected in status",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: corev1.ObjectReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
							)
							want.SetConditions(condition.ReconcileSuccess())
							diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty())
							want.SetConditions(condition.ReconcileError(errors.Wrap(errBoom, errExecutePosthooks)))
							diffPost := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty())
							if diff != "" && diffPost != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s, \n%s", diff, diffPost)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
					WithPosthook("preHookSuccess", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{}, nil
					})),
					WithPosthook("preHookFailed", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, errBoom
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"FailedPreAndPostHook": {
			reason: "Rendered workloads should be reflected in status",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileError(errors.Wrap(errBoom, errExecutePrehooks))),
							)
							diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty())
							want.SetConditions(condition.ReconcileError(errors.Wrap(errBoom, errExecutePosthooks)))
							diffPost := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty())
							if diff != "" && diffPost != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s, \n%s", diff, diffPost)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(obj client.Object) error {
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus,
						_ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
					WithPrehook("preHookSuccess", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, nil
					})),
					WithPrehook("preHookFailed", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, errBoom
					})),
					WithPosthook("preHookSuccess", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{}, nil
					})),
					WithPosthook("preHookFailed", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{RequeueAfter: 15 * time.Second}, errBoom
					})),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"SuccessWithHooks": {
			reason: "Rendered workloads should be reflected in status",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:    mockGetAppConfigFn,
						MockDelete: test.NewMockDeleteFn(nil),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: corev1.ObjectReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusPatch: test.NewMockSubResourcePatchFn(nil, func(o client.Object) error {
							want := ac(
								withConditions(condition.ReconcileSuccess()),
								withWorkloadStatuses(v1alpha2.WorkloadStatus{
									ComponentName: componentName,
									Reference: corev1.ObjectReference{
										APIVersion: workload.GetAPIVersion(),
										Kind:       workload.GetKind(),
										Name:       workload.GetName(),
									},
								}),
							)
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Status().Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						return []Workload{{ComponentName: componentName, Workload: workload}}, &v1alpha2.DependencyStatus{}, nil
					})),
					WithApplicator(WorkloadApplyFns{ApplyFn: func(_ context.Context, _ []v1alpha2.WorkloadStatus, _ []Workload, _ ...apply.ApplyOption) error {
						return nil
					}}),
					WithGarbageCollector(GarbageCollectorFn(func(_ string, _ []v1alpha2.WorkloadStatus, _ []Workload) []unstructured.Unstructured {
						return []unstructured.Unstructured{*trait}
					})),
					WithPrehook("preHook", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{}, nil
					})),
					WithPosthook("postHook", ControllerHooksFn(func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) (reconcile.Result, error) {
						return reconcile.Result{}, nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{RequeueAfter: 0},
			},
		},
		"RegisterFinalizer": {
			reason: "Register finalizer successfully",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
							o, _ := obj.(*v1alpha2.ApplicationConfiguration)
							*o = v1alpha2.ApplicationConfiguration{
								Spec: v1alpha2.ApplicationConfigurationSpec{
									Components: []v1alpha2.ApplicationConfigurationComponent{
										{
											ComponentName: componentName,
											Scopes: []v1alpha2.ComponentScope{
												{
													ScopeReference: corev1.ObjectReference{
														APIVersion: "core.oam.dev/v1alpha2",
														Kind:       "HealthScope",
														Name:       "example-healthscope",
													},
												},
											},
										},
									},
								},
							}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil, func(o client.Object) error {
							want := ac()
							if diff := cmp.Diff(want.GetFinalizers(), o.(*v1alpha2.ApplicationConfiguration).GetFinalizers(), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					},
				},
				o: []ReconcilerOption{
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"FinalizerSuccess": {
			reason: "",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
							o, _ := obj.(*v1alpha2.ApplicationConfiguration)
							*o = v1alpha2.ApplicationConfiguration{ObjectMeta: metav1.ObjectMeta{
								DeletionTimestamp: &now,
								Finalizers:        []string{workloadScopeFinalizer},
							}}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil, func(o client.Object) error {
							want := &v1alpha2.ApplicationConfiguration{ObjectMeta: metav1.ObjectMeta{
								DeletionTimestamp: &now,
								Finalizers:        []string{},
							}}
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					},
				},
				o: []ReconcilerOption{
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"FinalizerGetError": {
			reason: "",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
							o, _ := obj.(*v1alpha2.ApplicationConfiguration)
							*o = v1alpha2.ApplicationConfiguration{ObjectMeta: metav1.ObjectMeta{
								DeletionTimestamp: &now,
								Finalizers:        []string{workloadScopeFinalizer},
							}}
							return nil
						},
						MockUpdate: test.NewMockUpdateFn(nil, func(o client.Object) error {
							want := &v1alpha2.ApplicationConfiguration{ObjectMeta: metav1.ObjectMeta{
								DeletionTimestamp: &now,
								Finalizers:        []string{workloadScopeFinalizer},
							}}
							if diff := cmp.Diff(want, o.(*v1alpha2.ApplicationConfiguration), cmpopts.EquateEmpty()); diff != "" {
								t.Errorf("\nclient.Update(): -want, +got:\n%s", diff)
								return errUnexpectedStatus
							}
							return nil
						}),
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					},
				},
				o: []ReconcilerOption{
					WithApplicator(WorkloadApplyFns{FinalizeFn: func(ctx context.Context, ac *v1alpha2.ApplicationConfiguration) error {
						return errBoom
					}}),
					WithDependCheckWait(10 * time.Second),
				},
			},
			want: want{
				result: reconcile.Result{},
			},
		},
		"Without components": {
			reason: "should not crash when components is empty",
			args: args{
				m: &mock.Manager{
					Client: &test.MockClient{
						MockGet:          mockGetAppConfigFn,
						MockStatusUpdate: test.NewMockSubResourceUpdateFn(nil),
					},
				},
				o: []ReconcilerOption{
					WithRenderer(ComponentRenderFn(func(_ context.Context, _ *v1alpha2.ApplicationConfiguration) ([]Workload, *v1alpha2.DependencyStatus, error) {
						// without components
						return []Workload{}, &v1alpha2.DependencyStatus{}, nil
					})),
				},
			},
			want: want{
				result: reconcile.Result{},
				err:    nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, nil, tc.args.o...)
			got, err := r.Reconcile(context.TODO(), reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\n%s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestWorkloadStatus(t *testing.T) {
	namespace := "ns"
	componentName := "coolcomponent"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	cases := map[string]struct {
		w    Workload
		want v1alpha2.WorkloadStatus
	}{
		"Success": {
			w: Workload{
				ComponentName: componentName,
				Workload:      workload,
				Traits:        []*Trait{{Object: *trait}},
			},
			want: v1alpha2.WorkloadStatus{
				ComponentName: componentName,
				Reference: corev1.ObjectReference{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
				Traits: []v1alpha2.WorkloadTrait{
					{
						Reference: corev1.ObjectReference{
							APIVersion: trait.GetAPIVersion(),
							Kind:       trait.GetKind(),
							Name:       trait.GetName(),
						},
					},
				},
				Scopes: []v1alpha2.WorkloadScope{},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := tc.w.Status()
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("\nw.Status(): -want, +got:\n%s\n", diff)
			}
		})
	}

}

func TestEligible(t *testing.T) {
	namespace := "ns"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("v")
	workload.SetKind("workload")
	workload.SetNamespace(namespace)
	workload.SetName("workload")

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("v")
	trait.SetKind("trait")
	trait.SetNamespace(namespace)
	trait.SetName("trait")

	type args struct {
		namespace string
		ws        []v1alpha2.WorkloadStatus
		w         []Workload
	}
	cases := map[string]struct {
		reason string
		args   args
		want   []unstructured.Unstructured
	}{
		"TraitNotApplied": {
			reason: "A referenced trait is eligible for garbage collection if it was not applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: corev1.ObjectReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
				w: []Workload{{Workload: workload}},
			},
			want: []unstructured.Unstructured{*trait},
		},
		"NeitherApplied": {
			reason: "A referenced workload and its trait is eligible for garbage collection if they were not applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: corev1.ObjectReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
			},
			want: []unstructured.Unstructured{*workload, *trait},
		},
		"BothApplied": {
			reason: "A referenced workload and its trait are not eligible for garbage collection if they were applied",
			args: args{
				namespace: namespace,
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: corev1.ObjectReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: trait.GetAPIVersion(),
									Kind:       trait.GetKind(),
									Name:       trait.GetName(),
								},
							},
						},
					},
				},
				w: []Workload{{Workload: workload, Traits: []*Trait{{Object: *trait}}}},
			},
			want: []unstructured.Unstructured{},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := eligible(tc.args.namespace, tc.args.ws, tc.args.w)
			if diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("\n%s\neligible(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestIsRevisionWorkload(t *testing.T) {
	if true != IsRevisionWorkload(v1alpha2.WorkloadStatus{ComponentName: "compName", Reference: corev1.ObjectReference{Name: "compName-rev1"}}, nil) {
		t.Error("workloadName has componentName as prefix is revisionWorkload")
	}
	if true != IsRevisionWorkload(v1alpha2.WorkloadStatus{ComponentName: "compName", Reference: corev1.ObjectReference{Name: "speciedName"}}, []Workload{{ComponentName: "compName", RevisionEnabled: true}}) {
		t.Error("workloadName has componentName same and revisionEnabled is revisionWorkload")
	}
	if false != IsRevisionWorkload(v1alpha2.WorkloadStatus{ComponentName: "compName", Reference: corev1.ObjectReference{Name: "speciedName"}}, []Workload{{ComponentName: "compName", RevisionEnabled: false}}) {
		t.Error("workloadName has componentName same and revisionEnabled is false")
	}
	if false != IsRevisionWorkload(v1alpha2.WorkloadStatus{ComponentName: "compName", Reference: corev1.ObjectReference{Name: "speciedName"}}, []Workload{{ComponentName: "compName-notmatch", RevisionEnabled: true}}) {
		t.Error("workload with no prefix and no componentName match is not revisionEnabled ")
	}
}

func TestDependency(t *testing.T) {
	unreadyWorkload := &unstructured.Unstructured{}
	unreadyWorkload.SetAPIVersion("v1")
	unreadyWorkload.SetKind("Workload")
	unreadyWorkload.SetNamespace("test-ns")
	unreadyWorkload.SetName("unready-workload")

	readyWorkload := unreadyWorkload.DeepCopy()
	readyWorkload.SetName("ready-workload")
	err := unstructured.SetNestedField(readyWorkload.Object, "test", "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	readyWorkloadArrayField := unreadyWorkload.DeepCopy()
	err = unstructured.SetNestedStringSlice(readyWorkloadArrayField.Object, []string{"a"}, "spec", "key")
	if err != nil {
		t.Fatal(err)
	}
	err = unstructured.SetNestedStringSlice(readyWorkloadArrayField.Object, []string{"b"}, "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	unreadyTrait := &unstructured.Unstructured{}
	unreadyTrait.SetAPIVersion("v1")
	unreadyTrait.SetKind("Trait")
	unreadyTrait.SetNamespace("test-ns")
	unreadyTrait.SetName("unready-trait")

	readyTrait := unreadyTrait.DeepCopy()
	readyTrait.SetName("ready-trait")
	err = unstructured.SetNestedField(readyTrait.Object, "test", "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	refConfigMap := &unstructured.Unstructured{}
	refConfigMap.SetAPIVersion("v1")
	refConfigMap.SetKind("ConfigMap")
	refConfigMap.SetNamespace("test-ns")
	refConfigMap.SetName("ref-configmap")

	err = unstructured.SetNestedField(refConfigMap.Object, "test", "status", "key")
	if err != nil {
		t.Fatal(err)
	}

	readyDataPassingOutput := v1alpha2.DataOutput{
		Name: "test-ready-dataoutput",
		OutputStore: v1alpha2.StoreReference{
			ObjectReference: corev1.ObjectReference{
				APIVersion: refConfigMap.GetAPIVersion(),
				Kind:       refConfigMap.GetKind(),
				Name:       refConfigMap.GetName(),
			},
			Operations: []v1alpha2.DataOperation{{
				Conditions: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionNotEqual,
					Value:     "",
					FieldPath: "status.key",
				}},
			}},
		},
	}
	unreadyDataPassingOutput := v1alpha2.DataOutput{
		Name: "test-unready-dataoutput",
		OutputStore: v1alpha2.StoreReference{
			ObjectReference: corev1.ObjectReference{
				APIVersion: refConfigMap.GetAPIVersion(),
				Kind:       refConfigMap.GetKind(),
				Name:       refConfigMap.GetName(),
			},
			Operations: []v1alpha2.DataOperation{{
				Conditions: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "",
					FieldPath: "status.key",
				}},
			}},
		},
	}

	readyDataPassingInput := v1alpha2.DataInput{
		InputStore: v1alpha2.StoreReference{
			ObjectReference: corev1.ObjectReference{
				APIVersion: refConfigMap.GetAPIVersion(),
				Kind:       refConfigMap.GetKind(),
				Name:       refConfigMap.GetName(),
			},
			Operations: []v1alpha2.DataOperation{{
				Conditions: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionNotEqual,
					Value:     "",
					FieldPath: "status.key",
				}},
			}},
		},
	}
	unreadyDataPassingInput := v1alpha2.DataInput{
		InputStore: v1alpha2.StoreReference{
			ObjectReference: corev1.ObjectReference{
				APIVersion: refConfigMap.GetAPIVersion(),
				Kind:       refConfigMap.GetKind(),
				Name:       refConfigMap.GetName(),
			},
			Operations: []v1alpha2.DataOperation{{
				Conditions: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "",
					FieldPath: "status.key",
				}},
			}},
		},
	}

	mapper := mock.NewMockDiscoveryMapper()

	type args struct {
		components []v1alpha2.ApplicationConfigurationComponent
		wl         *unstructured.Unstructured
		trait      *unstructured.Unstructured
		refObj     *unstructured.Unstructured
	}
	type want struct {
		err             error
		verifyWorkloads func([]Workload)
		depStatus       *v1alpha2.DependencyStatus
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"Workload depends on another Workload that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].HasDep {
						t.Error("Workload should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "status.key not found in object",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Workload depends on another Workload that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    readyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].HasDep {
						t.Error("Workload should be ready to apply")
					}

					s, _, err := unstructured.NestedString(ws[0].Workload.UnstructuredContent(), "spec", "key")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(s, "test"); diff != "" {
						t.Fatal(diff)
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Workload depends on a Trait that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].HasDep {
						t.Error("Workload should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "status.key not found in object",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Workload depends on a Trait that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: readyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].HasDep {
						t.Error("Workload should be ready to apply")
					}

					s, _, err := unstructured.NestedString(ws[0].Workload.UnstructuredContent(), "spec", "key")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(s, "test"); diff != "" {
						t.Fatal(diff)
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Trait depends on a Workload that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].Traits[0].HasDep {
						t.Error("Trait should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "status.key not found in object",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyWorkload.GetAPIVersion(),
								Kind:       unreadyWorkload.GetKind(),
								Name:       unreadyWorkload.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Trait depends on a Workload that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    readyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].Traits[0].HasDep {
						t.Error("Trait should be ready to apply")
					}

					s, _, err := unstructured.NestedString(ws[0].Traits[0].Object.UnstructuredContent(), "spec", "key")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(s, "test"); diff != "" {
						t.Fatal(diff)
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			},
		},
		"Trait depends on another Trait that's unready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}, {
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if !ws[0].Traits[0].HasDep {
						t.Error("Trait should be unready to apply")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "status.key not found in object",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPath: "status.key",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: unreadyTrait.GetAPIVersion(),
								Kind:       unreadyTrait.GetKind(),
								Name:       unreadyTrait.GetName(),
							},
							FieldPaths: []string{"spec.key"},
						},
					}},
				},
			},
		},
		"Trait depends on another Trait that's ready": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					Traits: []v1alpha2.ComponentTrait{{
						Trait: runtime.RawExtension{},
						DataInputs: []v1alpha2.DataInput{{
							ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
							ToFieldPaths: []string{"spec.key"},
						}},
					}, {
						Trait: runtime.RawExtension{},
						DataOutputs: []v1alpha2.DataOutput{{
							Name:      "test-output",
							FieldPath: "status.key",
						}},
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: readyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].Traits[0].HasDep {
						t.Error("Trait should be ready to apply")
					}

					s, _, err := unstructured.NestedString(ws[0].Traits[0].Object.UnstructuredContent(), "spec", "key")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(s, "test"); diff != "" {
						t.Fatal(diff)
					}
				},
				depStatus: &v1alpha2.DependencyStatus{},
			}},
		"DataOutputName doesn't exist": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "wrong-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}},
				wl:    unreadyWorkload.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				err: ErrDataOutputNotExist,
			},
		},
		"DataInput of array type should append": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs: []v1alpha2.DataInput{{
						ValueFrom:    v1alpha2.DataInputValueFrom{DataOutputName: "test-output"},
						ToFieldPaths: []string{"spec.key"},
					}},
				}, {
					ComponentName: "test-component-source",
					DataOutputs: []v1alpha2.DataOutput{{
						Name:      "test-output",
						FieldPath: "status.key",
					}},
				}},
				wl:    readyWorkloadArrayField.DeepCopy(),
				trait: unreadyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					if ws[0].HasDep {
						t.Error("Workload should be ready to apply")
					}

					l, _, err := unstructured.NestedStringSlice(ws[0].Workload.UnstructuredContent(), "spec", "key")
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(l, []string{"a", "b"}); diff != "" {
						t.Fatal(diff)
					}
				},
				depStatus: &v1alpha2.DependencyStatus{}},
		},
		// add test cases for data passing in oam
		"DataOutputs with both ready and unready conditions": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs:    []v1alpha2.DataInput{},
				}, {
					ComponentName: "test-component-source",
					DataOutputs:   []v1alpha2.DataOutput{unreadyDataPassingOutput, readyDataPassingOutput},
				}},
				wl:    readyWorkload.DeepCopy(),
				trait: readyTrait.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					var wl Workload
					for i := range ws {
						if ws[i].ComponentName == "test-component-source" {
							wl = ws[i]
						}
					}
					if len(wl.DataOutputs) != 1 {
						t.Error("DataOutputs of workload should only contain readyDataPassingOutput")
					}
					if !reflect.DeepEqual(wl.DataOutputs[readyDataPassingOutput.Name], readyDataPassingOutput) {
						t.Error("DataOutputs of workload should contain readyDataPassingOutput")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "got(test) expected to be ",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: readyWorkload.GetAPIVersion(),
								Kind:       readyWorkload.GetKind(),
								Name:       readyWorkload.GetName(),
							},
							FieldPath: "",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: refConfigMap.GetAPIVersion(),
								Kind:       refConfigMap.GetKind(),
								Name:       refConfigMap.GetName(),
							},
							FieldPaths: []string{""},
						},
					}},
				},
			},
		},
		"DataInputs with only ready conditions": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs:    []v1alpha2.DataInput{readyDataPassingInput},
				}, {
					ComponentName: "test-component-source",
					DataOutputs:   []v1alpha2.DataOutput{unreadyDataPassingOutput, readyDataPassingOutput},
				}},
				wl:     readyWorkload.DeepCopy(),
				trait:  readyTrait.DeepCopy(),
				refObj: refConfigMap.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					var wl Workload
					for i := range ws {
						if ws[i].ComponentName == "test-component-sink" {
							wl = ws[i]
						}
					}
					if !reflect.DeepEqual(wl.DataInputs, []v1alpha2.DataInput{readyDataPassingInput}) {
						t.Error("DataInputs of workload should contain readyDataPassingInput")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "got(test) expected to be ",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: readyWorkload.GetAPIVersion(),
								Kind:       readyWorkload.GetKind(),
								Name:       readyWorkload.GetName(),
							},
							FieldPath: "",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: refConfigMap.GetAPIVersion(),
								Kind:       refConfigMap.GetKind(),
								Name:       refConfigMap.GetName(),
							},
							FieldPaths: []string{""},
						},
					}},
				},
			},
		},
		"DataInputs with both ready and unready conditions": {
			args: args{
				components: []v1alpha2.ApplicationConfigurationComponent{{
					ComponentName: "test-component-sink",
					DataInputs:    []v1alpha2.DataInput{unreadyDataPassingInput, readyDataPassingInput},
				}, {
					ComponentName: "test-component-source",
					DataOutputs:   []v1alpha2.DataOutput{readyDataPassingOutput},
				}},
				wl:     readyWorkload.DeepCopy(),
				trait:  readyTrait.DeepCopy(),
				refObj: refConfigMap.DeepCopy(),
			},
			want: want{
				verifyWorkloads: func(ws []Workload) {
					var wl Workload
					for i := range ws {
						if ws[i].ComponentName == "test-component-sink" {
							wl = ws[i]
						}
					}
					if len(wl.DataInputs) != 0 {
						t.Error("DataInputs of workload should contain no DataInput")
					}
				},
				depStatus: &v1alpha2.DependencyStatus{
					Unsatisfied: []v1alpha2.UnstaifiedDependency{{
						Reason: "got(test) expected to be ",
						From: v1alpha2.DependencyFromObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: refConfigMap.GetAPIVersion(),
								Kind:       refConfigMap.GetKind(),
								Name:       refConfigMap.GetName(),
							},
							FieldPath: "",
						},
						To: v1alpha2.DependencyToObject{
							ObjectReference: corev1.ObjectReference{
								APIVersion: readyWorkload.GetAPIVersion(),
								Kind:       readyWorkload.GetKind(),
								Name:       readyWorkload.GetName(),
							},
							FieldPaths: []string{""},
						},
					}},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := components{
				dm: mapper,
				client: &test.MockClient{
					MockGet: test.MockGetFn(func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
						if obj.GetObjectKind().GroupVersionKind().Kind == "Workload" {
							b, err := json.Marshal(tc.args.wl)
							if err != nil {
								t.Fatal(err)
							}
							err = json.Unmarshal(b, obj)
							if err != nil {
								t.Fatal(err)
							}
						}
						if obj.GetObjectKind().GroupVersionKind().Kind == "Trait" {
							b, err := json.Marshal(tc.args.trait)
							if err != nil {
								t.Fatal(err)
							}
							err = json.Unmarshal(b, obj)
							if err != nil {
								t.Fatal(err)
							}
						}
						if obj.GetObjectKind().GroupVersionKind().Kind == "ConfigMap" {
							b, err := json.Marshal(tc.args.refObj)
							if err != nil {
								t.Fatal(err)
							}
							err = json.Unmarshal(b, obj)
							if err != nil {
								t.Fatal(err)
							}
						}
						return nil
					}),
				},
				params: ParameterResolveFn(resolve),
				workload: ResourceRenderFn(func(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
					return tc.args.wl, nil
				}),
				trait: ResourceRenderFn(func(data []byte, p ...Parameter) (*unstructured.Unstructured, error) {
					return tc.args.trait, nil
				}),
			}

			ac := &v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "test-ns",
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: tc.args.components,
				},
			}

			ws, ds, err := c.Render(context.Background(), ac)
			if err != nil {
				if errors.Is(err, tc.want.err) {
					return
				}
				t.Error(err)
				return
			}
			if diff := cmp.Diff(tc.want.err, err); diff != "" {
				t.Error(diff)
				return
			}
			tc.want.verifyWorkloads(ws)
			if diff := cmp.Diff(tc.want.depStatus, ds); diff != "" {
				t.Error(diff)
			}
		})
	}
}

func TestAddDataOutputsToDAG(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("TestKind")
	obj.SetNamespace("test-ns")
	obj.SetName("test-name")

	dag := newDAG()
	outs := []v1alpha2.DataOutput{{
		Name:      "test-output",
		FieldPath: "spec.replica",
		Conditions: []v1alpha2.ConditionRequirement{{
			Operator:  v1alpha2.ConditionEqual,
			Value:     "abc",
			FieldPath: "status.state",
		}},
	}}
	addDataOutputsToDAG(dag, outs, obj)

	s, ok := dag.Sources["test-output"]
	if !ok {
		t.Fatal("didn't add source correctly")
	}

	r := &corev1.ObjectReference{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		FieldPath:  outs[0].FieldPath,
	}

	if diff := cmp.Diff(s.ObjectRef, r); diff != "" {
		t.Errorf("didn't add objectRef to source correctly: %s", diff)
	}

	if diff := cmp.Diff(s.Conditions, outs[0].Conditions); diff != "" {
		t.Errorf("didn't add conditions to source correctly: %s", diff)
	}
}

func TestPatchExtraField(t *testing.T) {
	tests := map[string]struct {
		acStatus      *v1alpha2.ApplicationConfigurationStatus
		acPatchStatus v1alpha2.ApplicationConfigurationStatus
		wantedStatus  *v1alpha2.ApplicationConfigurationStatus
	}{
		"patch extra": {
			acStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
			acPatchStatus: v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						Status:                "we need to add this",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Status: "add this too",
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
			wantedStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						Status:                "we need to add this",
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Status: "add this too",
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
		},
		"patch trait mismatch": {
			acStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
			acPatchStatus: v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						Status:                "we need to add this",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Status: "add this too",
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait2",
								},
							},
						},
					},
				},
			},
			wantedStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						Status:                "we need to add this",
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
		},
		"patch workload revision mismatch": {
			acStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
			acPatchStatus: v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						Status:                "we need to add this",
						ComponentRevisionName: "test-v2",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Status: "add this too",
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
			wantedStatus: &v1alpha2.ApplicationConfigurationStatus{
				Workloads: []v1alpha2.WorkloadStatus{
					{
						ComponentName:         "test",
						ComponentRevisionName: "test-v1",
						Traits: []v1alpha2.WorkloadTrait{
							{
								Reference: corev1.ObjectReference{
									APIVersion: "apiVersion1",
									Kind:       "kind1",
									Name:       "trait1",
								},
							},
						},
					},
				},
			},
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			patchExtraStatusField(tt.acStatus, tt.acPatchStatus)
			if diff := cmp.Diff(tt.acStatus, tt.wantedStatus); diff != "" {
				t.Errorf("didn't patch to the statsu correctly: %s", diff)
			}

		})
	}
}

func TestUpdateStatus(t *testing.T) {

	mockGetAppConfigFn := func(_ context.Context, key client.ObjectKey, obj client.Object) error {
		if o, ok := obj.(*v1alpha2.ApplicationConfiguration); ok {
			*o = v1alpha2.ApplicationConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "example-appconfig",
					Generation: 1,
				},
				Spec: v1alpha2.ApplicationConfigurationSpec{
					Components: []v1alpha2.ApplicationConfigurationComponent{
						{
							ComponentName: "example-component",
							ParameterValues: []v1alpha2.ComponentParameterValue{
								{
									Name: "image",
									Value: intstr.IntOrString{
										StrVal: "wordpress:php7.3",
									},
								},
							},
						},
					},
				},
				Status: v1alpha2.ApplicationConfigurationStatus{
					ObservedGeneration: 0,
				},
			}
		}
		return nil
	}

	m := &mock.Manager{
		Client: &test.MockClient{
			MockGet: mockGetAppConfigFn,
		},
	}

	r := NewReconciler(m, nil)

	ac := &v1alpha2.ApplicationConfiguration{}
	err := r.client.Get(context.Background(), types.NamespacedName{Name: "example-appconfig"}, ac)
	assert.Equal(t, err, nil)
	assert.Equal(t, ac.Status.ObservedGeneration, int64(0))

	updateObservedGeneration(ac)
	assert.Equal(t, ac.Status.ObservedGeneration, int64(1))

}
