/*
Copyright 2020 The Crossplane Authors.

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

package applicationconfiguration

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
)

func TestApplyWorkloads(t *testing.T) {
	errBoom := errors.New("boom")
	namespace := "ns"

	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("workload.oam.dev")
	workload.SetKind("workloadKind")
	workload.SetNamespace(namespace)
	workload.SetName("workload-example")
	workload.SetUID(types.UID("workload-uid"))

	trait := &unstructured.Unstructured{}
	trait.SetAPIVersion("trait.oam.dev")
	trait.SetKind("traitKind")
	trait.SetNamespace(namespace)
	trait.SetName("trait-example")
	trait.SetUID(types.UID("trait-uid"))

	scope, _ := util.Object2Unstructured(&v1alpha2.HealthScope{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-example",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scope.oam.dev/v1alpha2",
			Kind:       "scopeKind",
		},
		Spec: v1alpha2.HealthScopeSpec{
			// set an empty ref to enable wrokloadRefs field
			WorkloadReferences: []v1alpha1.TypedReference{
				{
					APIVersion: "",
					Kind:       "",
					Name:       "",
					UID:        "",
				},
			},
		},
	})

	// scope with Ref
	scopeWithRef, _ := util.Object2Unstructured(&v1alpha2.HealthScope{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-example",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scope.oam.dev/v1alpha2",
			Kind:       "scopeKind",
		},
		Spec: v1alpha2.HealthScopeSpec{
			WorkloadReferences: []v1alpha1.TypedReference{
				{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
			},
		},
	})

	scopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ScopeDefinition",
			APIVersion: "scopeDef.oam.dev",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-example.scope.oam.dev",
			Namespace: namespace,
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: "scope-example.scope.oam.dev",
			},
			WorkloadRefsPath: "spec.workloadRefs",
		},
	}

	type args struct {
		ctx context.Context
		ws  []v1alpha2.WorkloadStatus
		w   []Workload
	}

	cases := map[string]struct {
		reason         string
		client         resource.Applicator
		updatingClient resource.Applicator
		rawClient      client.Client
		args           args
		want           error
	}{
		"ApplyWorkloadError": {
			reason: "Errors applying a workload should be reflected as a status condition",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error {
				if w, ok := o.(*unstructured.Unstructured); ok && w.GetUID() == workload.GetUID() {
					return errBoom
				}
				return nil
			}),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient:      nil,
			args: args{
				w:  []Workload{{Workload: workload, Traits: []*Trait{{Object: *trait}}}},
				ws: []v1alpha2.WorkloadStatus{}},
			want: errors.Wrapf(errBoom, errFmtApplyWorkload, workload.GetName()),
		},
		"ApplyTraitError": {
			reason: "Errors applying a trait should be reflected as a status condition",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error {
				if t, ok := o.(*unstructured.Unstructured); ok && t.GetUID() == trait.GetUID() {
					return errBoom
				}
				return nil
			}),
			rawClient: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
			args: args{
				w:  []Workload{{Workload: workload, Traits: []*Trait{{Object: *trait}}}},
				ws: []v1alpha2.WorkloadStatus{}},
			want: errors.Wrapf(errBoom, errFmtApplyTrait, trait.GetAPIVersion(), trait.GetKind(), trait.GetName()),
		},
		"Success": {
			reason: "Applied workloads and traits should be returned as a set of UIDs.",
			client: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error {
				if o.GetObjectKind().GroupVersionKind().Kind == trait.GetKind() {
					// check that the trait should not have a workload ref since we didn't return a special traitDefinition
					obj, _ := util.Object2Map(o)
					if _, ok := obj["spec"]; ok {
						return fmt.Errorf("should not get workload ref on %q", obj["kind"])
					}
				}
				return nil
			}),
			rawClient: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
			args: args{
				w:  []Workload{{Workload: workload, Traits: []*Trait{{Object: *trait}}}},
				ws: []v1alpha2.WorkloadStatus{},
			},
		},
		"SuccessWithScope": {
			reason:         "Applied workloads refs to scopes.",
			client:         resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					if scopeDef, ok := obj.(*v1alpha2.ScopeDefinition); ok {
						*scopeDef = scopeDefinition
						return nil
					}
					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []*Trait{{Object: *trait.DeepCopy()}},
					Scopes:   []unstructured.Unstructured{*scope.DeepCopy()},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
		"SuccessWithScopeNoOp": {
			reason:         "Scope already has workloadRef.",
			client:         resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					if scopeDef, ok := obj.(*v1alpha2.ScopeDefinition); ok {
						*scopeDef = scopeDefinition
						return nil
					}
					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return fmt.Errorf("update is not expected in this test")
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []*Trait{{Object: *trait.DeepCopy()}},
					Scopes:   []unstructured.Unstructured{*scopeWithRef.DeepCopy()},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
		"SuccessRemoving": {
			reason:         "Removes workload refs from scopes.",
			client:         resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			updatingClient: resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
					if key.Name == scope.GetName() {
						scope := obj.(*unstructured.Unstructured)

						refs := []interface{}{
							map[string]interface{}{
								"apiVersion": workload.GetAPIVersion(),
								"kind":       workload.GetKind(),
								"name":       workload.GetName(),
							},
						}

						if err := fieldpath.Pave(scope.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err == nil {
							return err
						}

						return nil
					}
					if scopeDef, ok := obj.(*v1alpha2.ScopeDefinition); ok {
						*scopeDef = scopeDefinition
						return nil
					}
					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			args: args{
				w: []Workload{{
					Workload: workload,
					Traits:   []*Trait{{Object: *trait.DeepCopy()}},
					Scopes:   []unstructured.Unstructured{},
				}},
				ws: []v1alpha2.WorkloadStatus{
					{
						Reference: v1alpha1.TypedReference{
							APIVersion: workload.GetAPIVersion(),
							Kind:       workload.GetKind(),
							Name:       workload.GetName(),
						},
						Scopes: []v1alpha2.WorkloadScope{
							{
								Reference: v1alpha1.TypedReference{
									APIVersion: scope.GetAPIVersion(),
									Kind:       scope.GetKind(),
									Name:       scope.GetName(),
								},
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			mapper := mock.NewMockDiscoveryMapper()

			w := workloads{patchingClient: tc.client, updatingClient: tc.updatingClient, rawClient: tc.rawClient, dm: mapper}
			err := w.Apply(tc.args.ctx, tc.args.ws, tc.args.w)

			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nw.Apply(...): -want error, +got error:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestFinalizeWorkloadScopes(t *testing.T) {
	namespace := "ns"
	errMock := errors.New("mock error")
	workload := &unstructured.Unstructured{}
	workload.SetAPIVersion("workload.oam.dev")
	workload.SetKind("workloadKind")
	workload.SetNamespace(namespace)
	workload.SetName("workload-example")
	workload.SetUID(types.UID("workload-uid"))

	ctx := context.Background()

	scope, _ := util.Object2Unstructured(&v1alpha2.HealthScope{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-example",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "scope.oam.dev/v1alpha2",
			Kind:       "scopeKind",
		},
		Spec: v1alpha2.HealthScopeSpec{
			WorkloadReferences: []v1alpha1.TypedReference{
				{
					APIVersion: workload.GetAPIVersion(),
					Kind:       workload.GetKind(),
					Name:       workload.GetName(),
				},
			},
		},
	})
	scopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ScopeDefinition",
			APIVersion: "scopeDef.oam.dev",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scope-example.scope.oam.dev",
			Namespace: namespace,
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: "scope-example.scope.oam.dev",
			},
			WorkloadRefsPath: "spec.workloadRefs",
		},
	}

	ac := v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Finalizers: []string{workloadScopeFinalizer},
		},
		Status: v1alpha2.ApplicationConfigurationStatus{
			Workloads: []v1alpha2.WorkloadStatus{
				{
					Reference: v1alpha1.TypedReference{
						APIVersion: workload.GetAPIVersion(),
						Kind:       workload.GetKind(),
						Name:       workload.GetName(),
					},
					Scopes: []v1alpha2.WorkloadScope{
						{
							Reference: v1alpha1.TypedReference{
								APIVersion: scope.GetAPIVersion(),
								Kind:       scope.GetKind(),
								Name:       scope.GetName(),
							},
						},
					},
				},
			},
		},
	}

	cases := []struct {
		caseName       string
		client         resource.Applicator
		rawClient      client.Client
		wantErr        error
		wantFinalizers []string
	}{
		{
			caseName: "Finalization successes",
			client:   resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					if key.Name == scope.GetName() {
						scope := obj.(*unstructured.Unstructured)

						refs := []interface{}{
							map[string]interface{}{
								"apiVersion": workload.GetAPIVersion(),
								"kind":       workload.GetKind(),
								"name":       workload.GetName(),
							},
						}

						if err := fieldpath.Pave(scope.UnstructuredContent()).SetValue("spec.workloadRefs", refs); err == nil {
							return err
						}

						return nil
					}
					if scopeDef, ok := obj.(*v1alpha2.ScopeDefinition); ok {
						*scopeDef = scopeDefinition
						return nil
					}

					return nil
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			wantErr:        nil,
			wantFinalizers: []string{},
		},
		{
			caseName: "Finalization fails for error",
			client:   resource.ApplyFn(func(_ context.Context, o runtime.Object, _ ...resource.ApplyOption) error { return nil }),
			rawClient: &test.MockClient{
				MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
					return errMock
				},
				MockUpdate: func(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
					return nil
				},
			},
			wantErr:        errors.Wrapf(errMock, errFmtApplyScope, scope.GetAPIVersion(), scope.GetKind(), scope.GetName()),
			wantFinalizers: []string{workloadScopeFinalizer},
		},
	}
	for _, tc := range cases {
		t.Run(tc.caseName, func(t *testing.T) {
			acTest := ac
			mapper := mock.NewMockDiscoveryMapper()
			w := workloads{patchingClient: tc.client, rawClient: tc.rawClient, dm: mapper}
			err := w.Finalize(ctx, &acTest)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nw.Apply(...): -want error, +got error:\n%s", tc.caseName, diff)
			}
			if diff := cmp.Diff(tc.wantFinalizers, acTest.ObjectMeta.Finalizers); diff != "" {
				t.Errorf("\n%s\nw.Apply(...): -want error, +got error:\n%s", tc.caseName, diff)
			}
		})
	}

}
