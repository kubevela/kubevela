package apply

import (
	"context"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		existing   runtime.Object
		creatorErr error
		patcherErr error
		desired    runtime.Object
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
					func(ctx context.Context, existing, desired runtime.Object) error {
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
				creator: creatorFn(func(_ context.Context, _ client.Client, _ runtime.Object, _ ...ApplyOption) (runtime.Object, error) {
					return tc.args.existing, tc.args.creatorErr
				}),
				patcher: patcherFn(func(c, m runtime.Object) (client.Patch, error) {
					return nil, tc.args.patcherErr
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
		desired runtime.Object
		ao      []ApplyOption
	}
	type want struct {
		existing runtime.Object
		err      error
	}

	cases := map[string]struct {
		reason string
		c      client.Client
		args   args
		want   want
	}{
		"NotAMetadataObject": {
			reason: "An error should be returned if cannot access metadata of the desired object",
			args: args{
				desired: &testNoMetaObject{},
			},
			want: want{
				existing: nil,
				err:      errors.New("cannot access object metadata"),
			},
		},
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
					func(ctx context.Context, existing, desired runtime.Object) error {
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
					func(ctx context.Context, existing, desired runtime.Object) error {
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
				MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
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
			result, err := createOrGetExisting(ctx, tc.c, tc.args.desired, tc.args.ao...)
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
	ctx := context.TODO()

	cases := map[string]struct {
		reason  string
		u       types.UID
		current runtime.Object
		want    error
	}{
		"NoExistingObject": {
			reason: "No error should be returned if no existing object",
		},
		"Adoptable": {
			reason:  "A current object with no controller reference is not controllable",
			u:       uid,
			current: &testObject{},
			want:    errors.Errorf("existing object is not controlled by UID %q", uid),
		},
		"ControlledBySuppliedUID": {
			reason: "A current object that is already controlled by the supplied UID is controllable",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{{
				UID:        uid,
				Controller: pointer.BoolPtr(true),
			}}}},
		},
		"ControlledBySomeoneElse": {
			reason: "A current object that is already controlled by a different UID is not controllable",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{
				{
					UID:        types.UID("some-other-uid"),
					Controller: pointer.BoolPtr(true),
				},
			}}},
			want: errors.Errorf("existing object is not controlled by UID %q", uid),
		},
		"SharedControlledBySomeoneElse": {
			reason: "An object that has a shared controlled by a different UID is controllable",
			u:      uid,
			current: &testObject{ObjectMeta: metav1.ObjectMeta{OwnerReferences: []metav1.OwnerReference{
				{
					UID:        types.UID("some-other-uid"),
					Controller: pointer.BoolPtr(true),
				},
				{
					UID:        uid,
					Controller: pointer.BoolPtr(true),
				},
			}}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ao := MustBeControllableBy(tc.u)
			err := ao(ctx, nil, tc.current)
			if diff := cmp.Diff(tc.want, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nMustBeControllableBy(...)(...): -want error, +got error\n%s\n", tc.reason, diff)
			}
		})
	}
}
