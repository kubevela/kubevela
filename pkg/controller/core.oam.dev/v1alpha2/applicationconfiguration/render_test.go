/*
Copyright 2021 The Crossplane Authors.

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
	"fmt"
	"strconv"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamtype "github.com/oam-dev/kubevela/apis/types"
	helmapi "github.com/oam-dev/kubevela/pkg/appfile/helm/flux2apis"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/mock"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ ComponentRenderer = &components{}

func TestRender(t *testing.T) {
	errBoom := errors.New("boom")

	namespace := "ns"
	acName := "coolappconfig1"
	acUID := types.UID("definitely-a-uuid")
	componentName := "coolcomponent"
	workloadName := "coolworkload"
	traitName := "coolTrait"
	revisionName := "coolcomponent-v1"
	revisionName2 := "coolcomponent-v2"

	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Traits:        []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}

	revAC := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					RevisionName: revisionName,
					Traits:       []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}

	controlledTemplateAC := revAC.DeepCopy()
	controlledTemplateAC.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: v1beta1.SchemeGroupVersion.String(),
			Kind:       v1beta1.ApplicationKind,
			Controller: pointer.Bool(true),
		},
	}
	controlledTemplateAC.Spec.Components[0].RevisionName = revisionName2
	controlledTemplateAC.SetAnnotations(map[string]string{
		oam.AnnotationAppRollout:       strconv.FormatBool(true),
		oam.AnnotationRollingComponent: componentName,
		"keep":                         strconv.FormatBool(true),
	})
	controlledTemplatedAC := controlledTemplateAC.DeepCopy()
	controlledTemplatedAC.Status.RollingStatus = oamtype.RollingTemplated
	// ac will render template again if the status is not templated
	controlledForceTemplateAC := controlledTemplatedAC.DeepCopy()
	controlledForceTemplateAC.Status.RollingStatus = oamtype.RollingTemplating

	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)
	errTrait := errors.New("errTrait")

	type fields struct {
		client   client.Reader
		params   ParameterResolver
		workload ResourceRenderer
		trait    ResourceRenderer
	}
	type args struct {
		ac *v1alpha2.ApplicationConfiguration
	}
	type want struct {
		w   []Workload
		err error
	}
	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"GetError": {
			reason: "An error getting a component should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtGetComponent, componentName),
			},
		},
		"ResolveParamsError": {
			reason: "An error resolving the parameters of a component should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtResolveParams, componentName),
			},
		},
		"RenderWorkloadError": {
			reason: "An error rendering a component's workload should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtRenderWorkload, componentName),
			},
		},
		"RenderTraitError": {
			reason: "An error rendering a component's traits should be returned",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return &unstructured.Unstructured{}, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return nil, errBoom
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errBoom, errFmtRenderTrait, componentName),
			},
		},
		"GetTraitDefinitionError": {
			reason: "Errors getting a traitDefinition should be reflected as a status condition",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch robj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: revisionName2}}}
						ccomp.DeepCopyInto(robj)
					case *v1alpha2.TraitDefinition:
						return errTrait
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					return &unstructured.Unstructured{}, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					t.SetAPIVersion("traitAPI")
					t.SetKind("traitKind")
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				err: errors.Wrapf(errTrait, errFmtGetTraitDefinition, "traitAPI", "traitKind", traitName),
			},
		},
		"Success": {
			reason: "One workload and one trait should successfully be rendered",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					w.SetName(workloadName)
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName: componentName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(workloadName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							w.SetLabels(map[string]string{
								oam.LabelAppComponent:         componentName,
								oam.LabelAppName:              acName,
								oam.LabelAppComponentRevision: "",
								oam.LabelOAMResourceType:      oam.ResourceTypeWorkload,
							})
							return w
						}(),
						Traits: []*Trait{
							func() *Trait {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								t.SetAnnotations(map[string]string{
									oam.AnnotationAppGeneration: "0",
								})
								t.SetLabels(map[string]string{
									oam.LabelAppComponent:         componentName,
									oam.LabelAppName:              acName,
									oam.LabelAppComponentRevision: "",
									oam.LabelOAMResourceType:      oam.ResourceTypeTrait,
								})
								return &Trait{Object: *t, DataOutputs: make(map[string]v1alpha2.DataOutput)}
							}(),
						},
						Scopes:      []unstructured.Unstructured{},
						DataOutputs: make(map[string]v1alpha2.DataOutput),
					},
				},
			},
		},
		"Success-With-RevisionName": {
			reason: "Workload should successfully be rendered with fixed componentRevision",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					robj, ok := obj.(*v1.ControllerRevision)
					if ok {
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
								Status: v1alpha2.ComponentStatus{},
							}},
							Revision: 1,
						}
						rev.DeepCopyInto(robj)
						return nil
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: revAC},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(componentName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							w.SetLabels(map[string]string{
								oam.LabelAppComponent:         componentName,
								oam.LabelAppName:              acName,
								oam.LabelAppComponentRevision: revisionName,
								oam.LabelOAMResourceType:      oam.ResourceTypeWorkload,
							})
							return w
						}(),
						Traits: []*Trait{
							func() *Trait {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								t.SetAnnotations(map[string]string{
									oam.AnnotationAppGeneration: "0",
								})
								t.SetLabels(map[string]string{
									oam.LabelAppComponent:         componentName,
									oam.LabelAppName:              acName,
									oam.LabelAppComponentRevision: revisionName,
									oam.LabelOAMResourceType:      oam.ResourceTypeTrait,
								})
								return &Trait{Object: *t, DataOutputs: make(map[string]v1alpha2.DataOutput)}
							}(),
						},
						Scopes:      []unstructured.Unstructured{},
						DataOutputs: make(map[string]v1alpha2.DataOutput),
					},
				},
			},
		},
		"Success-With-RevisionEnabledTrait": {
			reason: "Workload name should successfully be rendered with revisionName",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch robj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: revisionName2}}}
						ccomp.DeepCopyInto(robj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName}, Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(robj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(revisionName2)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							w.SetLabels(map[string]string{
								oam.LabelAppComponent:         componentName,
								oam.LabelAppName:              acName,
								oam.LabelAppComponentRevision: revisionName2,
								oam.LabelOAMResourceType:      oam.ResourceTypeWorkload,
							})
							return w
						}(),
						Traits: []*Trait{
							func() *Trait {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetName(traitName)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								t.SetAnnotations(map[string]string{
									oam.AnnotationAppGeneration: "0",
								})
								t.SetLabels(map[string]string{
									oam.LabelAppComponent:         componentName,
									oam.LabelAppName:              acName,
									oam.LabelAppComponentRevision: revisionName2,
									oam.LabelOAMResourceType:      oam.ResourceTypeTrait,
								})
								return &Trait{Object: *t,
									Definition: v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: "coolTrait"}, Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}, DataOutputs: make(map[string]v1alpha2.DataOutput)}
							}(),
						},
						RevisionEnabled: true,
						Scopes:          []unstructured.Unstructured{},
						DataOutputs:     make(map[string]v1alpha2.DataOutput),
					},
				},
			},
		},
		"Success-With-WorkloadRef": {
			reason: "Workload should successfully be rendered with fixed componentRevision",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					robj, ok := obj.(*v1.ControllerRevision)
					if ok {
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
								Status: v1alpha2.ComponentStatus{},
							}},
							Revision: 1,
						}
						rev.DeepCopyInto(robj)
						return nil
					}
					trd, ok := obj.(*v1alpha2.TraitDefinition)
					if ok {
						td := v1alpha2.TraitDefinition{
							Spec: v1alpha2.TraitDefinitionSpec{
								WorkloadRefPath: "spec.workload.path",
							},
						}
						td.DeepCopyInto(trd)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					w.SetAPIVersion("traitApiVersion")
					w.SetKind("traitKind")
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: revAC},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetAPIVersion("traitApiVersion")
							w.SetKind("traitKind")
							w.SetNamespace(namespace)
							w.SetName(componentName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							w.SetLabels(map[string]string{
								oam.LabelAppComponent:         componentName,
								oam.LabelAppName:              acName,
								oam.LabelAppComponentRevision: revisionName,
								oam.LabelOAMResourceType:      oam.ResourceTypeWorkload,
							})
							return w
						}(),
						Traits: []*Trait{
							func() *Trait {
								tr := &unstructured.Unstructured{}
								tr.SetNamespace(namespace)
								tr.SetName(traitName)
								tr.SetOwnerReferences([]metav1.OwnerReference{*ref})
								tr.SetAnnotations(map[string]string{
									oam.AnnotationAppGeneration: "0",
								})
								tr.SetLabels(map[string]string{
									oam.LabelAppComponent:         componentName,
									oam.LabelAppName:              acName,
									oam.LabelAppComponentRevision: revisionName,
									oam.LabelOAMResourceType:      oam.ResourceTypeTrait,
								})
								workloadRef := corev1.ObjectReference{
									APIVersion: "traitApiVersion",
									Kind:       "traitKind",
									Name:       componentName,
								}
								if err := fieldpath.Pave(tr.Object).SetValue("spec.workload.path", workloadRef); err != nil {
									t.Fail()
								}
								return &Trait{Object: *tr, Definition: v1alpha2.TraitDefinition{Spec: v1alpha2.TraitDefinitionSpec{WorkloadRefPath: "spec.workload.path"}}, DataOutputs: make(map[string]v1alpha2.DataOutput)}
							}(),
						},
						Scopes:      []unstructured.Unstructured{},
						DataOutputs: make(map[string]v1alpha2.DataOutput),
					},
				},
			},
		},
		"Success-With-AppControlledAppConfig-CloneSet": {
			reason: "Workload name should be component name for CloneSet",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch defObj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{
							Status: v1alpha2.ComponentStatus{
								LatestRevision: &common.Revision{Name: revisionName2},
							},
						}
						ccomp.DeepCopyInto(defObj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName},
							Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(defObj)
					case *v1.ControllerRevision:
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec: v1alpha2.ComponentSpec{
									Workload: runtime.RawExtension{
										Object: &unstructured.Unstructured{},
									},
								},
								Status: v1alpha2.ComponentStatus{
									LatestRevision: &common.Revision{Name: revisionName2},
								},
							}},
							Revision: 2,
						}
						rev.DeepCopyInto(defObj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps.kruise.io/v1alpha1",
							"kind":       "CloneSet",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: controlledTemplateAC},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(componentName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							return w
						}(),
						RevisionEnabled: true,
					},
				},
			},
		},
		"Success-With-AppControlledAppConfig-Deployment": {
			reason: "Workload name should be component revision for Deployment",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch defObj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{
							Status: v1alpha2.ComponentStatus{
								LatestRevision: &common.Revision{Name: revisionName2},
							},
						}
						ccomp.DeepCopyInto(defObj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName},
							Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(defObj)
					case *v1.ControllerRevision:
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec: v1alpha2.ComponentSpec{
									Workload: runtime.RawExtension{
										Object: &unstructured.Unstructured{},
									},
								},
								Status: v1alpha2.ComponentStatus{
									LatestRevision: &common.Revision{Name: revisionName2},
								},
							}},
							Revision: 2,
						}
						rev.DeepCopyInto(defObj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: controlledTemplateAC},
			want: want{
				w: []Workload{
					{
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(revisionName2)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							return w
						}(),
						RevisionEnabled: true,
					},
				},
			},
		},
		"Success-With-Template-Finished-Deployment": {
			reason: "We do not render the workload after the template is done",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch defObj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{
							Status: v1alpha2.ComponentStatus{
								LatestRevision: &common.Revision{Name: revisionName2},
							},
						}
						ccomp.DeepCopyInto(defObj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName},
							Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(defObj)
					case *v1.ControllerRevision:
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec: v1alpha2.ComponentSpec{
									Workload: runtime.RawExtension{
										Object: &unstructured.Unstructured{},
									},
								},
								Status: v1alpha2.ComponentStatus{
									LatestRevision: &common.Revision{Name: revisionName2},
								},
							}},
							Revision: 2,
						}
						rev.DeepCopyInto(defObj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: controlledTemplatedAC},
			want: want{
				w: []Workload{
					{
						SkipApply:             true,
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(revisionName2)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							return w
						}(),
						RevisionEnabled: true,
					},
				},
			},
		},
		"Success-With-Force-Template-Deployment": {
			reason: "We force render the workload as long as the status is not templated",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
					switch defObj := obj.(type) {
					case *v1alpha2.Component:
						ccomp := v1alpha2.Component{
							Status: v1alpha2.ComponentStatus{
								LatestRevision: &common.Revision{Name: revisionName2},
							},
						}
						ccomp.DeepCopyInto(defObj)
					case *v1alpha2.TraitDefinition:
						ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName},
							Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
						ttrait.DeepCopyInto(defObj)
					case *v1.ControllerRevision:
						rev := &v1.ControllerRevision{
							ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
							Data: runtime.RawExtension{Object: &v1alpha2.Component{
								ObjectMeta: metav1.ObjectMeta{
									Name:      componentName,
									Namespace: namespace,
								},
								Spec: v1alpha2.ComponentSpec{
									Workload: runtime.RawExtension{
										Object: &unstructured.Unstructured{},
									},
								},
								Status: v1alpha2.ComponentStatus{
									LatestRevision: &common.Revision{Name: revisionName2},
								},
							}},
							Revision: 2,
						}
						rev.DeepCopyInto(defObj)
					}
					return nil
				})},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: args{ac: controlledForceTemplateAC},
			want: want{
				w: []Workload{
					{
						SkipApply:             false,
						ComponentName:         componentName,
						ComponentRevisionName: revisionName2,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(revisionName2)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							w.SetAnnotations(map[string]string{
								oam.AnnotationAppGeneration: "0",
							})
							return w
						}(),
						RevisionEnabled: true,
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &components{tc.fields.client, mock.NewMockDiscoveryMapper(), tc.fields.params,
				tc.fields.workload, tc.fields.trait}
			needTemplating := tc.args.ac.Status.RollingStatus != oamtype.RollingTemplated
			_, isRolling := tc.args.ac.GetAnnotations()[oam.AnnotationAppRollout]
			got, _, err := r.Render(context.Background(), tc.args.ac)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if isControlledByApp(tc.args.ac) {
				// test the case of application generated AC
				if diff := cmp.Diff(tc.want.w[0].ComponentName, got[0].ComponentName); diff != "" {
					t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
				}
				if diff := cmp.Diff(tc.want.w[0].ComponentRevisionName, got[0].ComponentRevisionName); diff != "" {
					t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
				}
				if diff := cmp.Diff(tc.want.w[0].Workload.GetName(), got[0].Workload.GetName()); diff != "" {
					t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
				}
				if _, exist := got[0].Workload.GetAnnotations()[oam.AnnotationAppRollout]; exist {
					t.Errorf("\n%s\nr.Render(...) workload should not get annotation:%s\n", tc.reason,
						oam.AnnotationAppRollout)
				}
				if _, exist := got[0].Workload.GetAnnotations()[oam.AnnotationRollingComponent]; exist {
					t.Errorf("\n%s\nr.Render(...) workload  should not get annotation:%s\n", tc.reason,
						oam.AnnotationRollingComponent)
				}
				if got[0].Workload.GetAnnotations()["keep"] != "true" {
					t.Errorf("\n%s\nr.Render(...) workload should get annotation:%s\n", tc.reason,
						"keep")
				}
				if _, exist := got[0].Traits[0].Object.GetAnnotations()[oam.AnnotationRollingComponent]; exist {
					t.Errorf("\n%s\nr.Render(...): trait should not get annotation:%s\n", tc.reason,
						oam.AnnotationRollingComponent)
				}
				if got[0].Traits[0].Object.GetAnnotations()["keep"] != "true" {
					t.Errorf("\n%s\nr.Render(...): trait should get annotation:%s\n", tc.reason,
						"keep")
				}
				if isRolling {
					if !needTemplating && !got[0].SkipApply {
						t.Errorf("\n%s\nr.Render(...): none template workload should be skip apply\n", tc.reason)
					}
					if needTemplating {
						if got[0].SkipApply {
							t.Errorf("\n%s\nr.Render(...): template workload should not be skipped\n", tc.reason)
						}
						if tc.args.ac.Status.RollingStatus != oamtype.RollingTemplating {
							t.Errorf("\n%s\nr.Render(...): ac status should be templated but got %s\n", tc.reason,
								tc.args.ac.Status.RollingStatus)
						}
					}

				}
			} else {
				if diff := cmp.Diff(tc.want.w, got); diff != "" {
					t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

func TestRenderComponent(t *testing.T) {
	type field struct {
		client   client.Reader
		workload ResourceRenderer
		trait    ResourceRenderer
	}
	type arg struct {
		ac                *v1alpha2.ApplicationConfiguration
		isControlledByApp bool
		isCompChanged     bool
		isRollingTemplate bool
		dag               *dag
	}
	type want struct {
		w   Workload
		err error
	}

	namespace := "ns"
	acName := "coolappconfig1"
	acUID := types.UID("definitely-a-uuid")
	componentName := "coolcomponent"
	traitName := "coolTrait"
	revisionName := "coolcomponent-v1"
	revisionName2 := "coolcomponent-v2"
	ctx := context.TODO()
	// Ac generated by application
	revAC := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					RevisionName: revisionName,
					Traits:       []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}
	ref := metav1.NewControllerRef(revAC, v1alpha2.ApplicationConfigurationGroupVersionKind)

	revAC2 := revAC.DeepCopy()
	revAC2.Spec.Components[0].RevisionName = revisionName2

	componentRevision := v1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
		Data: runtime.RawExtension{Object: &v1alpha2.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentName,
				Namespace: namespace,
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: runtime.RawExtension{
					Object: &unstructured.Unstructured{},
				},
			},
			Status: v1alpha2.ComponentStatus{
				LatestRevision: &common.Revision{Name: revisionName},
			},
		}},
	}

	mockGet := test.NewMockGetFn(nil, func(obj client.Object) error {
		switch defObj := obj.(type) {
		case *v1alpha2.TraitDefinition:
			ttrait := v1alpha2.TraitDefinition{ObjectMeta: metav1.ObjectMeta{Name: traitName},
				Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true}}
			ttrait.DeepCopyInto(defObj)
		case *v1.ControllerRevision:
			rev := &componentRevision
			rev.DeepCopyInto(defObj)
		}
		return nil
	})
	mockParams := ParameterResolveFn(func(_ []v1alpha2.ComponentParameter,
		_ []v1alpha2.ComponentParameterValue) ([]Parameter,
		error) {
		return nil, nil
	})
	cases := map[string]struct {
		reason string
		fields field
		args   arg
		want   want
	}{
		// TODO: Add more failure cases
		//  add more dependency related tests for any future changes
		//  add more trait related tests
		"Newly-Changed-Component-NotRolling": {
			reason: "newly changed workload should not be rendered if it's not rolling anymore (not the first time)",
			fields: field{
				client: &test.MockClient{MockGet: mockGet},

				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: arg{
				ac:                revAC,
				isControlledByApp: true,
				isCompChanged:     true,
				isRollingTemplate: false,
			},
			want: want{
				w: Workload{
					ComponentName:         componentName,
					ComponentRevisionName: revisionName,
					Workload: func() *unstructured.Unstructured {
						w := &unstructured.Unstructured{}
						w.SetName(revisionName)
						w.SetOwnerReferences([]metav1.OwnerReference{*ref})
						return w
					}(),
					RevisionEnabled: true,
				},
			},
		},
		"Success-With-Newly-Changed-Component-Deployment": {
			reason: "Workload name should be revision name for deployment and it should be disabled",
			fields: field{
				client: &test.MockClient{MockGet: mockGet},

				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: arg{
				ac:                revAC,
				isControlledByApp: true,
				isCompChanged:     true,
				isRollingTemplate: true,
			},
			want: want{
				w: Workload{
					ComponentName:         componentName,
					ComponentRevisionName: revisionName,
					Workload: func() *unstructured.Unstructured {
						w := &unstructured.Unstructured{}
						w.SetName(revisionName)
						w.SetOwnerReferences([]metav1.OwnerReference{*ref})
						unstructured.SetNestedField(w.Object, true, "spec", "paused")
						return w
					}(),
					RevisionEnabled: true,
				},
			},
		},
		"Success-With-Newly-Changed-Component-CloneSet": {
			reason: "Workload name should be component name for CloneSet and it should be disabled",
			fields: field{
				client: &test.MockClient{MockGet: mockGet},
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps.kruise.io/v1alpha1",
							"kind":       "CloneSet",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: arg{
				ac:                revAC2,
				isControlledByApp: true,
				isCompChanged:     true,
				isRollingTemplate: true,
			},
			want: want{
				w: Workload{
					ComponentName:         componentName,
					ComponentRevisionName: revisionName2,
					Workload: func() *unstructured.Unstructured {
						w := &unstructured.Unstructured{}
						w.SetName(componentName)
						w.SetOwnerReferences([]metav1.OwnerReference{*ref})
						unstructured.SetNestedField(w.Object, true, "spec", "updateStrategy", "paused")
						return w
					}(),
					RevisionEnabled: true,
				},
			},
		},
		"Success-With-NoChange-Component-Deployment": {
			reason: "Workload name should be revision name for deployment and it should be disabled",
			fields: field{
				client: &test.MockClient{MockGet: mockGet},
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "apps/v1",
							"kind":       "Deployment",
						},
					}
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					t.SetName(traitName)
					return t, nil
				}),
			},
			args: arg{
				ac:                revAC,
				isControlledByApp: true,
				isCompChanged:     false,
				isRollingTemplate: false,
			},
			want: want{
				w: Workload{
					SkipApply:             true,
					ComponentName:         componentName,
					ComponentRevisionName: revisionName,
					Workload: func() *unstructured.Unstructured {
						w := &unstructured.Unstructured{}
						w.SetName(revisionName)
						w.SetOwnerReferences([]metav1.OwnerReference{*ref})
						return w
					}(),
					RevisionEnabled: true,
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &components{tc.fields.client, mock.NewMockDiscoveryMapper(), mockParams,
				tc.fields.workload, tc.fields.trait}
			got, err := r.renderComponent(ctx, tc.args.ac.Spec.Components[0], tc.args.ac, tc.args.isControlledByApp,
				tc.args.isCompChanged, tc.args.isRollingTemplate, tc.args.dag)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.w.ComponentName, got.ComponentName); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.w.ComponentRevisionName, got.ComponentRevisionName); diff != "" {
				t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
			}
			if tc.args.isControlledByApp {
				if diff := cmp.Diff(tc.want.w.Workload.GetName(), got.Workload.GetName()); diff != "" {
					t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
				}
			}
			if tc.args.isCompChanged {
				if tc.args.isRollingTemplate {
					wantedSpec, exist, err := unstructured.NestedFieldCopy(tc.want.w.Workload.Object, "spec")
					assert.True(t, exist)
					assert.True(t, err == nil)
					gotSpec, exist, err := unstructured.NestedFieldCopy(got.Workload.Object, "spec")
					assert.True(t, exist)
					assert.True(t, err == nil)
					if diff := cmp.Diff(wantedSpec, gotSpec); diff != "" {
						t.Errorf("\n%s\nr.Render(...): -want, +got:\n%s\n", tc.reason, diff)
					}
				} else {
					// newly changed workloads are not rendered after the first time
					assert.True(t, got.SkipApply)
				}
			} else {
				// we won't touch the spec
				_, exist, err := unstructured.NestedFieldCopy(got.Workload.Object, "spec")
				assert.False(t, exist)
				assert.True(t, err == nil)
			}
		})
	}
}

func TestRenderWorkload(t *testing.T) {
	namespace := "ns"
	paramName := "coolparam"
	strVal := "coolstring"
	intVal := 32

	type args struct {
		data []byte
		p    []Parameter
	}
	type want struct {
		workload *unstructured.Unstructured
		err      error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnmarshalError": {
			reason: "Errors unmarshalling JSON should be returned",
			args: args{
				data: []byte(`wat`),
			},
			want: want{
				err: errors.Wrapf(errors.New("invalid character 'w' looking for beginning of value"), errUnmarshalWorkload),
			},
		},
		"SetStringError": {
			reason: "Errors setting a string value should be returned",
			args: args{
				data: []byte(`{"metadata":{}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromString(strVal),
					FieldPaths: []string{"metadata[0]"},
				}},
			},
			want: want{
				err: errors.Wrapf(errors.New("metadata is not an array"), errFmtSetParam, paramName),
			},
		},
		"SetNumberError": {
			reason: "Errors setting a number value should be returned",
			args: args{
				data: []byte(`{"metadata":{}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromInt(intVal),
					FieldPaths: []string{"metadata[0]"},
				}},
			},
			want: want{
				err: errors.Wrapf(errors.New("metadata is not an array"), errFmtSetParam, paramName),
			},
		},
		"Success": {
			reason: "A workload should be returned with the supplied parameters set",
			args: args{
				data: []byte(`{"metadata":{"namespace":"` + namespace + `","name":"name"}}`),
				p: []Parameter{{
					Name:       paramName,
					Value:      intstr.FromString(strVal),
					FieldPaths: []string{"metadata.name"},
				}},
			},
			want: want{
				workload: func() *unstructured.Unstructured {
					w := &unstructured.Unstructured{}
					w.SetNamespace(namespace)
					w.SetName(strVal)
					return w
				}(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := renderWorkload(tc.args.data, tc.args.p...)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nrenderWorkload(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.workload, got); diff != "" {
				t.Errorf("\n%s\nrenderWorkload(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRenderTrait(t *testing.T) {
	apiVersion := "coolversion"
	kind := "coolkind"

	type args struct {
		data []byte
	}
	type want struct {
		workload *unstructured.Unstructured
		err      error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"UnmarshalError": {
			reason: "Errors unmarshalling JSON should be returned",
			args: args{
				data: []byte(`wat`),
			},
			want: want{
				err: errors.Wrapf(errors.New("invalid character 'w' looking for beginning of value"), errUnmarshalTrait),
			},
		},
		"Success": {
			reason: "A workload should be returned with the supplied parameters set",
			args: args{
				data: []byte(`{"apiVersion":"` + apiVersion + `","kind":"` + kind + `"}`),
			},
			want: want{
				workload: func() *unstructured.Unstructured {
					w := &unstructured.Unstructured{}
					w.SetAPIVersion(apiVersion)
					w.SetKind(kind)
					return w
				}(),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := renderTrait(tc.args.data)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nrenderTrait(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.workload, got); diff != "" {
				t.Errorf("\n%s\nrenderTrait(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestResolveParams(t *testing.T) {
	paramName := "coolparam"
	required := true
	paths := []string{"metadata.name"}
	value := "cool"

	type args struct {
		cp  []v1alpha2.ComponentParameter
		cpv []v1alpha2.ComponentParameterValue
	}
	type want struct {
		p   []Parameter
		err error
	}
	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"MissingRequired": {
			reason: "An error should be returned when a required parameter is omitted",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name:     paramName,
						Required: &required,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{},
			},
			want: want{
				err: errors.Errorf(errFmtRequiredParam, paramName),
			},
		},
		"Unsupported": {
			reason: "An error should be returned when an unsupported parameter value is supplied",
			args: args{
				cp: []v1alpha2.ComponentParameter{},
				cpv: []v1alpha2.ComponentParameterValue{
					{
						Name: paramName,
					},
				},
			},
			want: want{
				err: errors.Errorf(errFmtUnsupportedParam, paramName),
			},
		},
		"MissingNotRequired": {
			reason: "Nothing should be returned when an optional parameter is omitted",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name: paramName,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{},
			},
			want: want{},
		},
		"SupportedAndSet": {
			reason: "A parameter should be returned when it is supported and set",
			args: args{
				cp: []v1alpha2.ComponentParameter{
					{
						Name:       paramName,
						FieldPaths: paths,
					},
				},
				cpv: []v1alpha2.ComponentParameterValue{
					{
						Name:  paramName,
						Value: intstr.FromString(value),
					},
				},
			},
			want: want{
				p: []Parameter{
					{
						Name:       paramName,
						FieldPaths: paths,
						Value:      intstr.FromString(value),
					},
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := resolve(tc.args.cp, tc.args.cpv)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nresolve(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.p, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("\n%s\nresolve(...): -want, +got:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestRenderTraitWithoutMetadataName(t *testing.T) {
	namespace := "ns"
	acName := "coolappconfig2"
	acUID := types.UID("definitely-a-uuid")
	componentName := "coolcomponent"
	workloadName := "coolworkload"

	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      acName,
			UID:       acUID,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: []v1alpha2.ApplicationConfigurationComponent{
				{
					ComponentName: componentName,
					Traits:        []v1alpha2.ComponentTrait{{}},
				},
			},
		},
	}

	ref := metav1.NewControllerRef(ac, v1alpha2.ApplicationConfigurationGroupVersionKind)

	type fields struct {
		client   client.Reader
		params   ParameterResolver
		workload ResourceRenderer
		trait    ResourceRenderer
	}
	type args struct {
		ac *v1alpha2.ApplicationConfiguration
	}
	type want struct {
		w []Workload
		// err error
	}
	cases := map[string]struct {
		reason string
		fields fields
		args   args
		want   want
	}{
		"Success": {
			reason: "One workload and one trait should successfully be rendered",
			fields: fields{
				client: &test.MockClient{MockGet: test.NewMockGetFn(nil)},
				params: ParameterResolveFn(func(_ []v1alpha2.ComponentParameter, _ []v1alpha2.ComponentParameterValue) ([]Parameter, error) {
					return nil, nil
				}),
				workload: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					w := &unstructured.Unstructured{}
					w.SetName(workloadName)
					return w, nil
				}),
				trait: ResourceRenderFn(func(_ []byte, _ ...Parameter) (*unstructured.Unstructured, error) {
					t := &unstructured.Unstructured{}
					return t, nil
				}),
			},
			args: args{ac: ac},
			want: want{
				w: []Workload{
					{
						ComponentName: componentName,
						Workload: func() *unstructured.Unstructured {
							w := &unstructured.Unstructured{}
							w.SetNamespace(namespace)
							w.SetName(workloadName)
							w.SetOwnerReferences([]metav1.OwnerReference{*ref})
							return w
						}(),
						Traits: []*Trait{
							func() *Trait {
								t := &unstructured.Unstructured{}
								t.SetNamespace(namespace)
								t.SetOwnerReferences([]metav1.OwnerReference{*ref})
								return &Trait{Object: *t}
							}(),
						},
					},
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := &components{tc.fields.client, mock.NewMockDiscoveryMapper(), tc.fields.params,
				tc.fields.workload, tc.fields.trait}
			got, _, _ := r.Render(context.Background(), tc.args.ac)
			if len(got) == 0 || len(got[0].Traits) == 0 || got[0].Traits[0].Object.GetName() != util.GenTraitName(componentName, ac.Spec.Components[0].Traits[0].DeepCopy(), "") {
				t.Errorf("\n%s\nr.Render(...): -want error, +got error:\n%s\n", tc.reason, "Trait name is NOT "+
					"automatically set.")
			}
		})
	}
}

func TestGetDefinitionName(t *testing.T) {

	tests := map[string]struct {
		u        *unstructured.Unstructured
		exp      string
		reason   string
		resource string
	}{
		"native resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
			}},
			exp:      "deployments.apps",
			reason:   "native resource match",
			resource: "deployments",
		},
		"extended resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "extend.oam.dev/v1alpha2",
				"kind":       "SimpleRolloutTrait",
			}},
			exp:      "simplerollouttraits.extend.oam.dev",
			reason:   "extend resource match",
			resource: "simplerollouttraits",
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			m := mock.NewMockDiscoveryMapper()
			m.MockRESTMapping = mock.NewMockRESTMapping(ti.resource)
			got, err := util.GetDefinitionName(m, ti.u, "")
			assert.NoError(t, err)
			if got != ti.exp {
				t.Errorf("%s getCRDName want %s got %s ", ti.reason, ti.exp, got)
			}
		})
	}
}

func TestSetWorkloadInstanceName(t *testing.T) {
	tests := map[string]struct {
		traitDefs       []v1alpha2.TraitDefinition
		u               *unstructured.Unstructured
		c               *v1alpha2.Component
		exp             *unstructured.Unstructured
		expErr          error
		currentWorkload *unstructured.Unstructured
		reason          string
	}{
		"with a name, no change": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "myname",
				},
			}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: "rev-1"}}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "myname",
				},
			}},
			reason: "workloadName should not change if already set",
		},
		"revisionEnabled true, and revision differs to the existing one, use new revisionName": {
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
				},
			},
			u: &unstructured.Unstructured{Object: map[string]interface{}{}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: "rev-2"}}},
			currentWorkload: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]string{
						"app.oam.dev/revision": "rev-v1",
					},
				},
			}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "rev-2",
				},
			}},
			reason: "workloadName should align with new revisionName",
		},
		"revisionEnabled true, and revision is same with the existing one, keep the old name": {
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: true},
				},
			},
			u: &unstructured.Unstructured{Object: map[string]interface{}{}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: "rev-1"}}},
			currentWorkload: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]string{
						"app.oam.dev/revision": "rev-v1",
					},
				},
			}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "rev-1",
				},
			}},
			reason: "workloadName should align with revisionName",
		},
		"revisionEnabled false, set componentName": {
			traitDefs: []v1alpha2.TraitDefinition{
				{
					Spec: v1alpha2.TraitDefinitionSpec{RevisionEnabled: false},
				},
			},
			u: &unstructured.Unstructured{Object: map[string]interface{}{}},
			c: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp"}, Status: v1alpha2.ComponentStatus{LatestRevision: &common.Revision{Name: "rev-1"}}},
			exp: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "comp",
				},
			}},
			reason: "workloadName should align with componentName",
		},
	}
	for name, ti := range tests {
		t.Run(name, func(t *testing.T) {
			if ti.expErr != nil {
				assert.Equal(t, ti.expErr.Error(), setWorkloadInstanceName(ti.traitDefs, ti.u, ti.c,
					ti.currentWorkload).Error())
			} else {
				err := setWorkloadInstanceName(ti.traitDefs, ti.u, ti.c, ti.currentWorkload)
				assert.NoError(t, err)
				assert.Equal(t, ti.exp, ti.u, ti.reason)
			}
		})
	}
}

func TestIsControlledByApp(t *testing.T) {
	// not true even if the kind checks right
	ac := &v1alpha2.ApplicationConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "acName",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "api",
					Kind:       v1alpha2.ApplicationKind,
				},
			},
		},
	}
	assert.False(t, isControlledByApp(ac))
	// not true even the owner type checks right
	ac.OwnerReferences = append(ac.OwnerReferences, metav1.OwnerReference{
		APIVersion: v1beta1.SchemeGroupVersion.String(),
		Kind:       v1beta1.ApplicationKind,
	})
	assert.False(t, isControlledByApp(ac))
	// only true when it's the controller
	ac.OwnerReferences[1].Controller = pointer.Bool(true)
	assert.True(t, isControlledByApp(ac))
}

func TestSetTraitProperties(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetName("hasName")
	setTraitProperties(u, "comp1", "ns", &metav1.OwnerReference{Name: "comp1"})
	expU := &unstructured.Unstructured{}
	expU.SetName("hasName")
	expU.SetNamespace("ns")
	expU.SetOwnerReferences([]metav1.OwnerReference{{Name: "comp1"}})
	assert.Equal(t, expU, u)

	u = &unstructured.Unstructured{}
	setTraitProperties(u, "comp1", "ns", &metav1.OwnerReference{Name: "comp1"})
	expU = &unstructured.Unstructured{}
	expU.SetName("comp1")
	expU.SetNamespace("ns")
	expU.SetOwnerReferences([]metav1.OwnerReference{{Name: "comp1"}})
	assert.Equal(t, expU, u)

	u = &unstructured.Unstructured{}
	u.SetOwnerReferences([]metav1.OwnerReference{
		{
			Name: "resourceTracker",
		},
	})
	u.SetNamespace("another-ns")
	setTraitProperties(u, "comp1", "ns", &metav1.OwnerReference{Name: "comp1"})
	expU = &unstructured.Unstructured{}
	expU.SetName("comp1")
	expU.SetNamespace("another-ns")
	expU.SetOwnerReferences([]metav1.OwnerReference{{Name: "resourceTracker"}})
	assert.Equal(t, expU, u)
}

func TestMatchValue(t *testing.T) {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("Workload")
	obj.SetNamespace("test-ns")
	obj.SetName("unready-workload")
	if err := unstructured.SetNestedField(obj.Object, "test", "key"); err != nil {
		t.Fatal(err)
	}
	paved, err := fieldpath.PaveObject(obj)
	if err != nil {
		t.Fatal(err)
	}

	ac := &unstructured.Unstructured{}
	ac.SetAPIVersion("core.oam.dev/v1alpha2")
	ac.SetKind("ApplicationConfiguration")
	ac.SetNamespace("test-ns")
	ac.SetName("test-app")
	if err := unstructured.SetNestedField(ac.Object, "test", "metadata", "labels", "app-hash"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(ac.Object, "test", "metadata", "labels", "app.hash"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(ac.Object, "different", "metadata", "annotations", "app-hash"); err != nil {
		t.Fatal(err)
	}
	if err := unstructured.SetNestedField(ac.Object, int64(123), "metadata", "annotations", "app-int"); err != nil {
		t.Fatal(err)
	}
	pavedAC, err := fieldpath.PaveObject(ac)
	if err != nil {
		t.Fatal(err)
	}

	type args struct {
		conds []v1alpha2.ConditionRequirement
		val   string
		paved *fieldpath.Paved
		ac    *fieldpath.Paved
	}
	type want struct {
		matched bool
		reason  string
	}
	cases := map[string]struct {
		args args
		want want
	}{
		"No conditions with empty value should not match": {
			want: want{
				matched: false,
				reason:  "value should not be empty",
			},
		},
		"No conditions with nonempty value should match": {
			args: args{
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with same value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionEqual,
					Value:    "test",
				}},
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with different value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionEqual,
					Value:    "test",
				}},
				val: "different",
			},
			want: want{
				matched: false,
				reason:  "got(different) expected to be test",
			},
		},
		"notEq condition with different value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEqual,
					Value:    "test",
				}},
				val: "different",
			},
			want: want{
				matched: true,
			},
		},
		"notEq condition with same value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEqual,
					Value:    "test",
				}},
				val: "test",
			},
			want: want{
				matched: false,
				reason:  "got(test) expected not to be test",
			},
		},
		"notEmpty condition with nonempty value should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEmpty,
				}},
				val: "test",
			},
			want: want{
				matched: true,
			},
		},
		"notEmpty condition with empty value should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator: v1alpha2.ConditionNotEmpty,
				}},
				val: "",
			},
			want: want{
				matched: false,
				reason:  "value should not be empty",
			},
		},
		"eq condition with same value from FieldPath should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "test",
					FieldPath: "key",
				}},
				paved: paved,
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with different value from FieldPath should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					Value:     "different",
					FieldPath: "key",
				}},
				paved: paved,
			},
			want: want{
				matched: false,
				reason:  "got(test) expected to be different",
			},
		},
		"eq condition with same value from FieldPath and valueFrom AppConfig should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels.app-hash"},
					FieldPath: "key",
				}},
				paved: paved,
				ac:    pavedAC,
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with same value but contain period in annotation should match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.labels[app.hash]"},
					FieldPath: "key",
				}},
				paved: paved,
				ac:    pavedAC,
			},
			want: want{
				matched: true,
			},
		},
		"eq condition with different value from FieldPath and valueFrom AppConfig should not match": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.annotations.app-hash"},
					FieldPath: "key",
				}},
				paved: paved,
				ac:    pavedAC,
			},
			want: want{
				matched: false,
				reason:  "got(test) expected to be different",
			},
		},
		"only string type is supported": {
			args: args{
				conds: []v1alpha2.ConditionRequirement{{
					Operator:  v1alpha2.ConditionEqual,
					ValueFrom: v1alpha2.ValueFrom{FieldPath: "metadata.annotations.app-int"},
					FieldPath: "key",
				}},
				paved: paved,
				ac:    pavedAC,
			},
			want: want{
				matched: false,
				reason:  "get valueFrom.fieldPath fail: metadata.annotations.app-int: not a string",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			matched, reason := matchValue(tc.args.conds, tc.args.val, tc.args.paved, tc.args.ac)
			if diff := cmp.Diff(tc.want.matched, matched); diff != "" {
				t.Error(diff)
			}
			assert.Equal(t, tc.want.reason, reason)
		})
	}
}

func TestDiscoverHelmModuleWorkload(t *testing.T) {
	ns := "test-ns"
	releaseName := "test-rls"
	chartName := "test-chart"
	release := &unstructured.Unstructured{}
	release.SetGroupVersionKind(helmapi.HelmReleaseGVK)
	release.SetName(releaseName)
	unstructured.SetNestedMap(release.Object, map[string]interface{}{
		"chart": map[string]interface{}{
			"spec": map[string]interface{}{
				"chart":   chartName,
				"version": "1.0.0",
			},
		},
	}, "spec")
	releaseRaw, _ := release.MarshalJSON()

	rlsWithoutChart := release.DeepCopy()
	unstructured.SetNestedMap(rlsWithoutChart.Object, nil, "spec", "chart")
	rlsWithoutChartRaw, _ := rlsWithoutChart.MarshalJSON()

	wl := &unstructured.Unstructured{}
	wl.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
	})
	wl.SetAnnotations(map[string]string{
		"meta.helm.sh/release-name":      releaseName,
		"meta.helm.sh/release-namespace": ns,
	})

	tests := map[string]struct {
		reason         string
		c              client.Reader
		helm           *common.Helm
		workloadInComp *unstructured.Unstructured
		wantWorkload   *unstructured.Unstructured
		wantErr        error
	}{
		"CompHasNoHelm": {
			reason:  "An error should occur because component has no Helm module",
			wantErr: errors.New("the component has no valid helm module"),
		},
		"CannotGetReleaseFromComp": {
			reason: "An error should occur because cannot get release",
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: []byte("boom")},
			},
			wantErr: errors.Wrap(errors.New("invalid character 'b' looking for beginning of value"),
				"cannot get helm release from component"),
		},
		"CannotGetChartFromRelease": {
			reason: "An error should occur because cannot get chart info",
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: rlsWithoutChartRaw},
			},
			wantErr: errors.New("cannot get helm chart name"),
		},
		"CannotGetWLFromComp": {
			reason: "An error should occur because cannot get workload from component",
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: releaseRaw},
			},
			wantErr: errors.Wrap(errors.New("unexpected end of JSON input"),
				"cannot get workload from component"),
		},
		"CannotGetWorkload": {
			reason: "An error should occur because cannot get workload from k8s cluster",
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: releaseRaw},
			},
			workloadInComp: &unstructured.Unstructured{},
			c:              &test.MockClient{MockGet: test.NewMockGetFn(errors.New("boom"))},
			wantErr:        errors.New("boom"),
		},
		"GetNotMatchedWorkload": {
			reason: "An error should occur because the found workload is not managed by Helm",
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: releaseRaw},
			},
			workloadInComp: &unstructured.Unstructured{},
			c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
				o, _ := obj.(*unstructured.Unstructured)
				*o = unstructured.Unstructured{}
				o.SetLabels(map[string]string{
					"app.kubernetes.io/managed-by": "non-helm",
				})
				return nil
			})},
			wantErr: fmt.Errorf("the workload is found but not match with helm info(meta.helm.sh/release-name: %s, meta.helm.sh/namespace: %s, app.kubernetes.io/managed-by: Helm)", "test-rls", "test-ns"),
		},
		"DiscoverSuccessfully": {
			reason: "No error should occur and the workload shoud be returned",
			c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
				o, _ := obj.(*unstructured.Unstructured)
				*o = *wl.DeepCopy()
				return nil
			})},
			workloadInComp: wl.DeepCopy(),
			helm: &common.Helm{
				Release: runtime.RawExtension{Raw: releaseRaw},
			},
			wantWorkload: wl.DeepCopy(),
			wantErr:      nil,
		},
	}

	for caseName, tc := range tests {
		t.Run(caseName, func(t *testing.T) {
			comp := &v1alpha2.Component{}
			if tc.workloadInComp != nil {
				wlRaw, _ := tc.workloadInComp.MarshalJSON()
				comp.Spec.Workload = runtime.RawExtension{Raw: wlRaw}
			}
			comp.Spec.Helm = tc.helm
			wl, err := discoverHelmModuleWorkload(context.Background(), tc.c, comp, ns)
			if diff := cmp.Diff(tc.wantWorkload, wl); diff != "" {
				t.Errorf("\n%s\ndiscoverHelmModuleWorkload(...)(...): -want object, +got object\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nApply(...): -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}
