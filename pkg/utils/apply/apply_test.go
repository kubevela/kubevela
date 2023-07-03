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

package apply

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/oam-dev/kubevela/pkg/oam"
)

var ctx = context.Background()
var errFake = errors.New("fake error")

type testObject struct {
	runtime.Object
	metav1.ObjectMeta
}

func (t *testObject) DeepCopyObject() runtime.Object {
	return &testObject{ObjectMeta: *t.ObjectMeta.DeepCopy()}
}

func (t *testObject) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

/*
	func TestCreator(t *testing.T) {
		desired := &unstructured.Unstructured{}
		desired.SetName("desired")
		type args struct {
			desired client.Object
			ao      []Option
		}
		type want struct {
			existing client.Object
			err      error
		}

		cases := map[string]struct {
			reason string
			c      client.Client
			args   args
			want   want
		}{
			"CannotCreateObjectWithoutName": {
				reason: "An error should be returned if cannot create the object",
				args: args{
					desired: &testObject{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "prefix",
						},
					},
				},
				c: &test.MockClient{MockCreate: test.NewMockCreateFn(errFake)},
				want: want{
					existing: nil,
					err:      errors.Wrap(errFake, "cannot create object"),
				},
			},
			"CannotCreate": {
				reason: "An error should be returned if cannot create the object",
				c: &test.MockClient{
					MockCreate: test.NewMockCreateFn(errFake),
					MockGet:    test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
				args: args{
					desired: desired,
				},
				want: want{
					existing: nil,
					err:      errors.Wrap(errFake, "cannot create object"),
				},
			},
			"CannotGetExisting": {
				reason: "An error should be returned if cannot get the object",
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(errFake)},
				args: args{
					desired: desired,
				},
				want: want{
					existing: nil,
					err:      errors.Wrap(errFake, "cannot get object"),
				},
			},
			"ApplyOptionErrorWhenCreatObjectWithoutName": {
				reason: "An error should be returned if cannot apply Option",
				args: args{
					desired: &testObject{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "prefix",
						},
					},
					ao: []Option{
						func(_ *applyAction, existing, desired client.Object) error {
							return errFake
						},
					},
				},
				want: want{
					existing: nil,
					err:      errors.Wrap(errFake, "cannot apply Option"),
				},
			},
			"ApplyOptionErrorWhenCreatObject": {
				reason: "An error should be returned if cannot apply Option",
				c:      &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
				args: args{
					desired: desired,
					ao: []Option{
						func(_ *applyAction, existing, desired client.Object) error {
							return errFake
						},
					},
				},
				want: want{
					existing: nil,
					err:      errors.Wrap(errFake, "cannot apply Option"),
				},
			},
			"CreateWithoutNameSuccessfully": {
				reason: "No error and existing should be returned if create successfully",
				c:      &test.MockClient{MockCreate: test.NewMockCreateFn(nil)},
				args: args{
					desired: &testObject{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "prefix",
						},
					},
				},
				want: want{
					existing: nil,
					err:      nil,
				},
			},
			"CreateSuccessfully": {
				reason: "No error and existing should be returned if create successfully",
				c: &test.MockClient{
					MockCreate: test.NewMockCreateFn(nil),
					MockGet:    test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
				args: args{
					desired: desired,
				},
				want: want{
					existing: nil,
					err:      nil,
				},
			},
			"GetExistingSuccessfully": {
				reason: "Existing object and no error should be returned",
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(obj client.Object) error {
						o, _ := obj.(*unstructured.Unstructured)
						*o = *desired
						return nil
					})},
				args: args{
					desired: desired,
				},
				want: want{
					existing: desired,
					err:      nil,
				},
			},
		}

		for caseName, tc := range cases {
			t.Run(caseName, func(t *testing.T) {
				act := new(applyAction)
				result, err := createOrGetExisting(ctx, act, tc.c, tc.args.desired, tc.args.ao...)
				if diff := cmp.Diff(tc.want.existing, result); diff != "" {
					t.Errorf("\n%s\ncreateOrGetExisting(...): -want , +got \n%s\n", tc.reason, diff)
				}
				if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
					t.Errorf("\n%s\ncreateOrGetExisting(...): -want error, +got error\n%s\n", tc.reason, diff)
				}
			})
		}

}

	func TestMustBeControlledByApp(t *testing.T) {
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app"}}
		ao := MustBeControlledByApp(app)
		testCases := map[string]struct {
			existing client.Object
			hasError bool
		}{
			"no old app": {
				existing: nil,
				hasError: false,
			},
			"old app has no label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{ResourceVersion: "-"}},
				hasError: true,
			},
			"old app has no app label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels:          map[string]string{},
					ResourceVersion: "-",
				}},
				hasError: true,
			},
			"old app has no app ns label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels:          map[string]string{oam.LabelAppName: "app"},
					ResourceVersion: "-",
				}},
				hasError: true,
			},
			"old app has correct label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "default"},
				}},
				hasError: false,
			},
			"old app has incorrect app label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{oam.LabelAppName: "a", oam.LabelAppNamespace: "default"},
				}},
				hasError: true,
			},
			"old app has incorrect ns label": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "ns"},
				}},
				hasError: true,
			},
			"old app has no resource version but with bad app key": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{oam.LabelAppName: "app", oam.LabelAppNamespace: "ns"},
				}},
				hasError: true,
			},
			"old app has no resource version": {
				existing: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				}},
				hasError: false,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				err := ao(&applyAction{}, tc.existing, nil)
				if tc.hasError {
					r.Error(err)
				} else {
					r.NoError(err)
				}
			})
		}
	}

	func TestSharedByApp(t *testing.T) {
		app := &v1beta1.Application{ObjectMeta: metav1.ObjectMeta{Name: "app"}}
		ao := SharedByApp(app)
		testCases := map[string]struct {
			existing client.Object
			desired  client.Object
			output   client.Object
			hasError bool
		}{
			"create new resource": {
				existing: nil,
				desired: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
				}},
				output: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "default/app"},
					},
				}},
			},
			"add sharer to existing resource": {
				existing: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
				}},
				desired: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
				}},
				output: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "default/app"},
					},
				}},
			},
			"add sharer to existing sharing resource": {
				existing: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "example",
							oam.LabelAppNamespace: "default",
						},
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "x/y"},
					},
					"data": "x",
				}},
				desired: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"data": "y",
				}},
				output: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "example",
							oam.LabelAppNamespace: "default",
						},
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "x/y,default/app"},
					},
					"data": "x",
				}},
			},
			"add sharer to existing sharing resource owned by self": {
				existing: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "app",
							oam.LabelAppNamespace: "default",
						},
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "default/app,x/y"},
					},
					"data": "x",
				}},
				desired: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "app",
							oam.LabelAppNamespace: "default",
						},
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "default/app"},
					},
					"data": "y",
				}},
				output: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "app",
							oam.LabelAppNamespace: "default",
						},
						"annotations": map[string]interface{}{oam.AnnotationAppSharedBy: "default/app,x/y"},
					},
					"data": "y",
				}},
			},
			"add sharer to existing non-sharing resource": {
				existing: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							oam.LabelAppName:      "example",
							oam.LabelAppNamespace: "default",
						},
					},
				}},
				desired: &unstructured.Unstructured{Object: map[string]interface{}{
					"kind": "ConfigMap",
				}},
				hasError: true,
			},
		}
		for name, tc := range testCases {
			t.Run(name, func(t *testing.T) {
				r := require.New(t)
				err := ao(&applyAction{}, tc.existing, tc.desired)
				if tc.hasError {
					r.Error(err)
				} else {
					r.NoError(err)
					r.Equal(tc.output, tc.desired)
				}
			})
		}
	}
*/
func TestFilterSpecialAnn(t *testing.T) {
	var cm = &corev1.ConfigMap{}
	var sc = &corev1.Secret{}
	var dp = &appsv1.Deployment{}
	var crd = &v1.CustomResourceDefinition{}
	assert.Equal(t, false, trimLastAppliedConfigurationForSpecialResources(cm))
	assert.Equal(t, false, trimLastAppliedConfigurationForSpecialResources(sc))
	assert.Equal(t, false, trimLastAppliedConfigurationForSpecialResources(crd))
	assert.Equal(t, true, trimLastAppliedConfigurationForSpecialResources(dp))

	dp.Annotations = map[string]string{oam.AnnotationLastAppliedConfig: "-"}
	assert.Equal(t, false, trimLastAppliedConfigurationForSpecialResources(dp))
	dp.Annotations = map[string]string{oam.AnnotationLastAppliedConfig: "skip"}
	assert.Equal(t, false, trimLastAppliedConfigurationForSpecialResources(dp))
	dp.Annotations = map[string]string{oam.AnnotationLastAppliedConfig: "xxx"}
	assert.Equal(t, true, trimLastAppliedConfigurationForSpecialResources(dp))
}
