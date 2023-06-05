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

package resourcekeeper

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
)

func Test_gcHandler_GarbageCollectApplicationRevision(t *testing.T) {
	type fields struct {
		resourceKeeper *resourceKeeper
		cfg            *gcConfig
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "cleanUpApplicationRevision and cleanUpWorkflowComponentRevision success",
			fields: fields{
				resourceKeeper: &resourceKeeper{
					Client: test.NewMockClient(),
					app:    &v1beta1.Application{},
				},
				cfg: &gcConfig{
					disableApplicationRevisionGC: false,
					disableComponentRevisionGC:   false,
				},
			},
		},
		{
			name: "failed",
			fields: fields{
				resourceKeeper: &resourceKeeper{
					Client: &test.MockClient{
						MockGet:         test.NewMockGetFn(errors.New("mock")),
						MockList:        test.NewMockListFn(errors.New("mock")),
						MockCreate:      test.NewMockCreateFn(errors.New("mock")),
						MockDelete:      test.NewMockDeleteFn(errors.New("mock")),
						MockDeleteAllOf: test.NewMockDeleteAllOfFn(errors.New("mock")),
						MockUpdate:      test.NewMockUpdateFn(errors.New("mock")),
						MockPatch:       test.NewMockPatchFn(errors.New("mock")),
					},
					app: &v1beta1.Application{},
				},
				cfg: &gcConfig{
					disableApplicationRevisionGC: false,
					disableComponentRevisionGC:   false,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &gcHandler{
				resourceKeeper: tt.fields.resourceKeeper,
				cfg:            tt.fields.cfg,
			}
			if err := h.GarbageCollectApplicationRevision(context.Background()); (err != nil) != tt.wantErr {
				t.Errorf("gcHandler.GarbageCollectApplicationRevision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cleanUpApplicationRevision(t *testing.T) {
	type args struct {
		h *gcHandler
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "clean up app-v2",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						Client: &test.MockClient{
							MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
								l, _ := list.(*v1beta1.ApplicationRevisionList)
								l.Items = []v1beta1.ApplicationRevision{
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v1",
										},
									},
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v2",
										},
									},
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v3",
										},
									},
								}
								return nil
							},
							MockDelete: test.NewMockDeleteFn(nil),
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								LatestRevision: &apicommon.Revision{
									Name: "app-v1",
								},
							},
						},
					},
					cfg: &gcConfig{
						disableApplicationRevisionGC: false,
						appRevisionLimit:             1,
					},
				},
			},
		},
		{
			name: "disabled",
			args: args{
				h: &gcHandler{
					cfg: &gcConfig{
						disableApplicationRevisionGC: true,
					},
				},
			},
		},
		{
			name: "list failed",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						Client: &test.MockClient{
							MockList: test.NewMockListFn(errors.New("mock")),
						},
						app: &v1beta1.Application{},
					},
					cfg: &gcConfig{
						disableApplicationRevisionGC: false,
						appRevisionLimit:             1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "delete failed",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						Client: &test.MockClient{
							MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
								l, _ := list.(*v1beta1.ApplicationRevisionList)
								l.Items = []v1beta1.ApplicationRevision{
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v1",
										},
									},
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v2",
										},
									},
									{
										ObjectMeta: metav1.ObjectMeta{
											Name: "app-v3",
										},
									},
								}
								return nil
							},
							MockDelete: test.NewMockDeleteFn(errors.New("mock")),
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								LatestRevision: &apicommon.Revision{
									Name: "app-v1",
								},
							},
						},
					},
					cfg: &gcConfig{
						disableApplicationRevisionGC: false,
						appRevisionLimit:             1,
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cleanUpApplicationRevision(context.Background(), tt.args.h); (err != nil) != tt.wantErr {
				t.Errorf("cleanUpApplicationRevision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cleanUpWorkflowComponentRevision(t *testing.T) {
	type args struct {
		h *gcHandler
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "clean up found revisions",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						_crRT: &v1beta1.ResourceTracker{},
						Client: &test.MockClient{
							MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
								if key.Name == "revision3" {
									return kerrors.NewNotFound(schema.GroupResource{}, "")
								}
								o, _ := obj.(*unstructured.Unstructured)
								o.SetLabels(map[string]string{
									oam.LabelAppComponentRevision: "revision1",
								})
								return nil
							},
							MockDelete: test.NewMockDeleteFn(nil),
							MockUpdate: test.NewMockUpdateFn(nil),
							MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
								l, _ := list.(*appsv1.ControllerRevisionList)
								l.Items = []appsv1.ControllerRevision{
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision1", Namespace: "default"},
										Revision:   1,
									},
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision2", Namespace: "default"},
										Revision:   2,
									},
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision3", Namespace: "default"},
										Revision:   3,
									},
								}
								return nil
							},
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								AppliedResources: []apicommon.ClusterObjectReference{
									{
										ObjectReference: corev1.ObjectReference{
											Namespace:  "default",
											Name:       "revision1",
											APIVersion: appsv1.SchemeGroupVersion.String(),
											Kind:       "Deployment",
										},
									},
									{
										ObjectReference: corev1.ObjectReference{
											Namespace:  "default",
											Name:       "revision3",
											APIVersion: appsv1.SchemeGroupVersion.String(),
											Kind:       "Deployment",
										},
									},
								},
							},
							ObjectMeta: metav1.ObjectMeta{}}},
					cfg: &gcConfig{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
		},
		{
			name: "no need clean up",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						_crRT: &v1beta1.ResourceTracker{},
						Client: &test.MockClient{
							MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
								o, _ := obj.(*unstructured.Unstructured)
								o.SetLabels(map[string]string{
									oam.LabelAppComponentRevision: "revision1",
								})
								return nil
							},
							MockDelete: test.NewMockDeleteFn(nil),
							MockUpdate: test.NewMockUpdateFn(nil),
							MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
								l, _ := list.(*appsv1.ControllerRevisionList)
								l.Items = []appsv1.ControllerRevision{
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision1", Namespace: "default"},
										Revision:   1,
									},
								}
								return nil
							},
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								AppliedResources: []apicommon.ClusterObjectReference{
									{},
								},
							},
							ObjectMeta: metav1.ObjectMeta{}}},
					cfg: &gcConfig{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
		},
		{
			name: "disabled",
			args: args{
				h: &gcHandler{
					cfg: &gcConfig{
						disableComponentRevisionGC: true,
					},
				},
			},
		},
		{
			name: "get failed",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						Client: &test.MockClient{
							MockGet: test.NewMockGetFn(errors.New("mock")),
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								AppliedResources: []apicommon.ClusterObjectReference{
									{},
								},
							},
							ObjectMeta: metav1.ObjectMeta{}}},
					cfg: &gcConfig{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "list failed",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						Client: &test.MockClient{
							MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
								o, _ := obj.(*unstructured.Unstructured)
								o.SetLabels(map[string]string{
									oam.LabelAppComponentRevision: "revision1",
								})
								return nil
							},
							MockList: test.NewMockListFn(errors.New("mock")),
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								AppliedResources: []apicommon.ClusterObjectReference{
									{},
								},
							},
							ObjectMeta: metav1.ObjectMeta{}}},
					cfg: &gcConfig{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "deleteComponentRevision failed",
			args: args{
				h: &gcHandler{
					resourceKeeper: &resourceKeeper{
						_crRT: &v1beta1.ResourceTracker{},
						Client: &test.MockClient{
							MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
								o, _ := obj.(*unstructured.Unstructured)
								o.SetLabels(map[string]string{
									oam.LabelAppComponentRevision: "revision1",
								})
								return nil
							},
							MockDelete: test.NewMockDeleteFn(errors.New("mock")),
							MockUpdate: test.NewMockUpdateFn(nil),
							MockList: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
								l, _ := list.(*appsv1.ControllerRevisionList)
								l.Items = []appsv1.ControllerRevision{
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision1", Namespace: "default"},
										Revision:   1,
									},
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revision2", Namespace: "default"},
										Revision:   2,
									},
									{
										ObjectMeta: metav1.ObjectMeta{Name: "revisio3", Namespace: "default"},
										Revision:   3,
									},
								}
								return nil
							},
						},
						app: &v1beta1.Application{
							Status: apicommon.AppStatus{
								AppliedResources: []apicommon.ClusterObjectReference{
									{},
								},
							},
							ObjectMeta: metav1.ObjectMeta{}}},
					cfg: &gcConfig{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cleanUpComponentRevision(context.Background(), tt.args.h); (err != nil) != tt.wantErr {
				t.Errorf("cleanUpWorkflowComponentRevision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
