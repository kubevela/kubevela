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

package appkeeper

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
	"github.com/oam-dev/kubevela/pkg/resourcekeeper"
	"github.com/oam-dev/kubevela/pkg/resourcetracker"
)

func TestApplicationRevisionsAreCollectedToTheLimit(t *testing.T) {
	type fields struct {
		resourceKeeper *resourceKeeperFixture
		cfg            *gcConfigFixture
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "cleanUpApplicationRevision and cleanUpWorkflowComponentRevision success",
			fields: fields{
				resourceKeeper: &resourceKeeperFixture{
					Client: test.NewMockClient(),
					app:    &v1beta1.Application{},
				},
				cfg: &gcConfigFixture{
					disableApplicationRevisionGC: false,
					disableComponentRevisionGC:   false,
				},
			},
		},
		{
			name: "failed",
			fields: fields{
				resourceKeeper: &resourceKeeperFixture{
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
				cfg: &gcConfigFixture{
					disableApplicationRevisionGC: false,
					disableComponentRevisionGC:   false,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &gcHandlerFixture{
				resourceKeeper: tt.fields.resourceKeeper,
				cfg:            tt.fields.cfg,
			}
			c, st := h.collector()
			if err := c.gcRevisions(context.Background(), st); (err != nil) != tt.wantErr {
				t.Errorf("gcRevisions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cleanUpApplicationRevision(t *testing.T) {
	type args struct {
		h *gcHandlerFixture
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "clean up app-v2",
			args: args{
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
						disableApplicationRevisionGC: false,
						appRevisionLimit:             1,
					},
				},
			},
		},
		{
			name: "disabled",
			args: args{
				h: &gcHandlerFixture{
					cfg: &gcConfigFixture{
						disableApplicationRevisionGC: true,
					},
				},
			},
		},
		{
			name: "list failed",
			args: args{
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
						Client: &test.MockClient{
							MockList: test.NewMockListFn(errors.New("mock")),
						},
						app: &v1beta1.Application{},
					},
					cfg: &gcConfigFixture{
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
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
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
			c, st := tt.args.h.collector()
			if err := c.cleanUpApplicationRevision(context.Background(), st); (err != nil) != tt.wantErr {
				t.Errorf("cleanUpApplicationRevision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_cleanUpWorkflowComponentRevision(t *testing.T) {
	type args struct {
		h *gcHandlerFixture
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "clean up found revisions",
			args: args{
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
		},
		{
			name: "no need clean up",
			args: args{
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
						disableComponentRevisionGC: false,
						appRevisionLimit:           1,
					},
				},
			},
		},
		{
			name: "disabled",
			args: args{
				h: &gcHandlerFixture{
					cfg: &gcConfigFixture{
						disableComponentRevisionGC: true,
					},
				},
			},
		},
		{
			name: "get failed",
			args: args{
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
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
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
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
				h: &gcHandlerFixture{
					resourceKeeper: &resourceKeeperFixture{
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
					cfg: &gcConfigFixture{
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
			c, st := tt.args.h.collector()
			if err := c.cleanUpComponentRevision(context.Background(), st); (err != nil) != tt.wantErr {
				t.Errorf("cleanUpComponentRevision() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Fixtures for the Application collector: the trackers a garbage-collection pass would have
// marked, as the CollectState the keeper hands to Options.Collect.
type resourceKeeperFixture struct {
	Client client.Client
	app    *v1beta1.Application
	_crRT  *v1beta1.ResourceTracker
}

type gcConfigFixture struct {
	disableApplicationRevisionGC bool
	disableComponentRevisionGC   bool
	appRevisionLimit             int
}

type gcHandlerFixture struct {
	resourceKeeper *resourceKeeperFixture
	cfg            *gcConfigFixture
}

func (h *gcHandlerFixture) collector() (*appCollector, resourcekeeper.CollectState) {
	if h.resourceKeeper == nil { // cases that return before touching the keeper
		h.resourceKeeper = &resourceKeeperFixture{app: &v1beta1.Application{}}
	}
	app := h.resourceKeeper.app
	c := &appCollector{cli: h.resourceKeeper.Client, app: app, tracked: NewAppResourceTracker(app), crRT: h.resourceKeeper._crRT}
	return c, resourcekeeper.CollectState{
		Trackers:                   resourcetracker.Trackers{ComponentRevision: h.resourceKeeper._crRT},
		RevisionLimit:              h.cfg.appRevisionLimit,
		DisableRevisionGC:          h.cfg.disableApplicationRevisionGC,
		DisableComponentRevisionGC: h.cfg.disableComponentRevisionGC,
	}
}
