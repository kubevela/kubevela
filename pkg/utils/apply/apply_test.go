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

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
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

type testNoMetaObject struct {
	runtime.Object
}

func TestAPIApplicator(t *testing.T) {
	existing := &testObject{}
	existing.SetName("existing")
	desired := &testObject{}
	desired.SetName("desired")
	// use Deployment as a registered API sample
	testDeploy := &appsv1.Deployment{}
	testDeploy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	})
	type args struct {
		existing   client.Object
		creatorErr error
		patcherErr error
		desired    client.Object
		ao         []ApplyOption
	}

	cases := map[string]struct {
		reason string
		c      client.Client
		args   args
		want   error
	}{
		"ErrorOccursCreatOrGetExisting": {
			reason: "An error should be returned if cannot create or get existing",
			args: args{
				creatorErr: errFake,
			},
			want: errFake,
		},
		"CreateSuccessfully": {
			reason: "No error should be returned if create successfully",
		},
		"CannotApplyApplyOptions": {
			reason: "An error should be returned if cannot apply ApplyOption",
			args: args{
				existing: existing,
				ao: []ApplyOption{
					func(_ *applyAction, existing, desired client.Object) error {
						return errFake
					},
				},
			},
			want: errors.Wrap(errFake, "cannot apply ApplyOption"),
		},
		"CalculatePatchError": {
			reason: "An error should be returned if patch failed",
			args: args{
				existing:   existing,
				desired:    desired,
				patcherErr: errFake,
			},
			c:    &test.MockClient{MockPatch: test.NewMockPatchFn(errFake)},
			want: errors.Wrap(errFake, "cannot calculate patch by computing a three way diff"),
		},
		"PatchError": {
			reason: "An error should be returned if patch failed",
			args: args{
				existing: existing,
				desired:  testDeploy,
			},
			c:    &test.MockClient{MockPatch: test.NewMockPatchFn(errFake)},
			want: errors.Wrap(errFake, "cannot patch object"),
		},
		"PatchingApplySuccessfully": {
			reason: "No error should be returned if patch successfully",
			args: args{
				existing: existing,
				desired:  desired,
			},
			c: &test.MockClient{MockPatch: test.NewMockPatchFn(nil)},
		},
	}

	for caseName, tc := range cases {
		t.Run(caseName, func(t *testing.T) {
			a := &APIApplicator{
				creator: creatorFn(func(_ context.Context, _ *applyAction, _ client.Client, _ client.Object, _ ...ApplyOption) (client.Object, error) {
					return tc.args.existing, tc.args.creatorErr
				}),
				patcher: patcherFn(func(c, m client.Object, a *applyAction) (client.Patch, error) {
					return client.RawPatch(types.MergePatchType, []byte(`err`)), tc.args.patcherErr
				}),
				c: tc.c,
			}
			result := a.Apply(ctx, tc.args.desired, tc.args.ao...)
			if diff := cmp.Diff(tc.want, result, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nApply(...): -want , +got \n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestCreator(t *testing.T) {
	desired := &unstructured.Unstructured{}
	desired.SetName("desired")
	type args struct {
		desired client.Object
		ao      []ApplyOption
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
			reason: "An error should be returned if cannot apply ApplyOption",
			args: args{
				desired: &testObject{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "prefix",
					},
				},
				ao: []ApplyOption{
					func(_ *applyAction, existing, desired client.Object) error {
						return errFake
					},
				},
			},
			want: want{
				existing: nil,
				err:      errors.Wrap(errFake, "cannot apply ApplyOption"),
			},
		},
		"ApplyOptionErrorWhenCreatObject": {
			reason: "An error should be returned if cannot apply ApplyOption",
			c:      &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
			args: args{
				desired: desired,
				ao: []ApplyOption{
					func(_ *applyAction, existing, desired client.Object) error {
						return errFake
					},
				},
			},
			want: want{
				existing: nil,
				err:      errors.Wrap(errFake, "cannot apply ApplyOption"),
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

func TestMustBeControllableBy(t *testing.T) {
	uid := types.UID("very-unique-string")
	controller := true

	cases := map[string]struct {
		reason  string
		current client.Object
		u       types.UID
		want    error
	}{
		"NoExistingObject": {
			reason: "No error should be returned if no existing object",
		},
		"Adoptable": {
			reason:  "A current object with no controller reference may be adopted and controlled",
			u:       uid,
			current: &testObject{},
		},
		"ControlledBySuppliedUID": {
			reason: "A current object that is already controlled by the supplied UID is controllable",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				UID:        uid,
				Controller: &controller,
			}}}},
		},
		"ControlledBySomeoneElse": {
			reason: "A current object that is already controlled by a different UID is not controllable",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				UID:        types.UID("some-other-uid"),
				Controller: &controller,
			}}}},
			want: errors.Errorf("existing object is not controlled by UID %q", uid),
		},
		"cross namespace resource": {
			reason: "A cross namespace resource have a resourceTracker owner, skip check UID",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				UID:        uid,
				Controller: &controller,
				Kind:       v1beta1.ResourceTrackerKind,
			}}}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ao := MustBeControllableBy(tc.u)
			act := new(applyAction)
			err := ao(act, tc.current, nil)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nMustBeControllableBy(...)(...): -want error, +got error\n%s\n", tc.reason, diff)
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
