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

package util_test

import (
	"context"
	"fmt"
	"hash/adler32"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/mock"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestUnstructured(t *testing.T) {
	tests := map[string]struct {
		u         *unstructured.Unstructured
		typeLabel string
		exp       string
		resource  string
	}{
		"native resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
			}},
			resource: "deployments",
			exp:      "deployments.apps",
		},
		"workload": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.WorkloadTypeLabel: "deploy",
					},
				},
			}},
			typeLabel: oam.WorkloadTypeLabel,
			exp:       "deploy",
		},
	}
	for name, ti := range tests {
		mapper := mock.NewClient(nil, nil).RESTMapper()
		got, err := util.GetDefinitionName(mapper, ti.u, ti.typeLabel)
		assert.NoError(t, err)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, ti.exp, got)
	}
}

func TestGetGVKFromDef(t *testing.T) {
	mapper := mock.NewClient(nil, map[schema.GroupVersionResource][]schema.GroupVersionKind{
		schema.GroupVersionResource{Group: "example.com", Resource: "abcs"}:                {{Group: "example.com", Version: "v1", Kind: "Abc"}},
		schema.GroupVersionResource{Group: "example.com", Resource: "abcs", Version: "v2"}: {{Group: "example.com", Version: "v2", Kind: "Abc"}},
	}).RESTMapper()
	gvk, err := util.GetGVKFromDefinition(mapper, common.DefinitionReference{Name: "abcs.example.com"})
	assert.NoError(t, err)
	assert.Equal(t, metav1.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Abc",
	}, gvk)

	gvk, err = util.GetGVKFromDefinition(mapper, common.DefinitionReference{Name: "abcs.example.com", Version: "v2"})
	assert.NoError(t, err)
	assert.Equal(t, metav1.GroupVersionKind{
		Group:   "example.com",
		Version: "v2",
		Kind:    "Abc",
	}, gvk)

	gvk, err = util.GetGVKFromDefinition(mapper, common.DefinitionReference{})
	assert.NoError(t, err)
	assert.Equal(t, metav1.GroupVersionKind{
		Group:   "",
		Version: "",
		Kind:    "",
	}, gvk)

	gvk, err = util.GetGVKFromDefinition(mapper, common.DefinitionReference{Name: "dummy"})
	assert.NoError(t, err)
	assert.Equal(t, metav1.GroupVersionKind{
		Group:   "",
		Version: "",
		Kind:    "",
	}, gvk)
}

func TestConvertWorkloadGVK2Def(t *testing.T) {
	mapper := mock.NewClient(nil, map[schema.GroupVersionResource][]schema.GroupVersionKind{}).RESTMapper()
	ref, err := util.ConvertWorkloadGVK2Definition(mapper, common.WorkloadGVK{APIVersion: "apps.kruise.io/v1alpha1",
		Kind: "CloneSet"})
	assert.NoError(t, err)
	assert.Equal(t, common.DefinitionReference{
		Name:    "clonesets.apps.kruise.io",
		Version: "v1alpha1",
	}, ref)

	ref, err = util.ConvertWorkloadGVK2Definition(mapper, common.WorkloadGVK{APIVersion: "apps/v1",
		Kind: "Deployment"})
	assert.NoError(t, err)
	assert.Equal(t, common.DefinitionReference{
		Name:    "deployments.apps",
		Version: "v1",
	}, ref)

	_, err = util.ConvertWorkloadGVK2Definition(mapper, common.WorkloadGVK{APIVersion: "/apps/v1",
		Kind: "Deployment"})
	assert.Error(t, err)
}

func TestDeepHashObject(t *testing.T) {
	successCases := []func() interface{}{
		func() interface{} { return 8675309 },
		func() interface{} { return "Jenny, I got your number" },
		func() interface{} { return []string{"eight", "six", "seven"} },
	}

	for _, tc := range successCases {
		hasher1 := adler32.New()
		util.DeepHashObject(hasher1, tc())
		hash1 := hasher1.Sum32()
		util.DeepHashObject(hasher1, tc())
		hash2 := hasher1.Sum32()

		assert.Equal(t, hash1, hash2)
	}
}

func TestEndReconcileWithNegativeCondition(t *testing.T) {

	var time1, time2 time.Time
	time1 = time.Now()
	time2 = time1.Add(time.Second)

	type args struct {
		ctx       context.Context
		r         client.StatusClient
		workload  util.ConditionedObject
		condition []condition.Condition
	}
	patchErr := fmt.Errorf("eww")
	tests := []struct {
		name     string
		args     args
		expected error
	}{
		{
			name: "no condition is added",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(nil),
				},
				workload:  &mock.Target{},
				condition: []condition.Condition{},
			},
			expected: nil,
		},
		{
			name: "condition is changed",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(nil),
				},
				workload: &mock.Target{
					ConditionedStatus: condition.ConditionedStatus{
						Conditions: []condition.Condition{
							{
								Type:               "test",
								LastTransitionTime: metav1.NewTime(time1),
								Reason:             "old reason",
								Message:            "old error msg",
							},
						},
					},
				},
				condition: []condition.Condition{
					{
						Type:               "test",
						LastTransitionTime: metav1.NewTime(time2),
						Reason:             "new reason",
						Message:            "new error msg",
					},
				},
			},
			expected: nil,
		},
		{
			name: "condition is not changed",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(nil),
				},
				workload: &mock.Target{
					ConditionedStatus: condition.ConditionedStatus{
						Conditions: []condition.Condition{
							{
								Type:               "test",
								LastTransitionTime: metav1.NewTime(time1),
								Reason:             "old reason",
								Message:            "old error msg",
							},
						},
					},
				},
				condition: []condition.Condition{
					{
						Type:               "test",
						LastTransitionTime: metav1.NewTime(time2),
						Reason:             "old reason",
						Message:            "old error msg",
					},
				},
			},
			expected: fmt.Errorf(util.ErrReconcileErrInCondition, "test", "old error msg"),
		},
		{
			name: "fail for patching error",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(patchErr),
				},
				workload: &mock.Target{},
				condition: []condition.Condition{
					{},
				},
			},
			expected: errors.Wrap(patchErr, util.ErrUpdateStatus),
		},
	}
	for _, tt := range tests {
		err := util.EndReconcileWithNegativeCondition(tt.args.ctx, tt.args.r, tt.args.workload, tt.args.condition...)
		if tt.expected == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tt.expected.Error(), err.Error())
		}
	}
}

func TestEndReconcileWithPositiveCondition(t *testing.T) {
	type args struct {
		ctx       context.Context
		r         client.StatusClient
		workload  util.ConditionedObject
		condition []condition.Condition
	}
	patchErr := fmt.Errorf("eww")
	tests := []struct {
		name     string
		args     args
		expected error
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(nil),
				},
				workload: &mock.Target{},
				condition: []condition.Condition{
					{},
				},
			},
			expected: nil,
		},
		{
			name: "fail",
			args: args{
				ctx: context.Background(),
				r: &test.MockClient{
					MockStatusPatch: test.NewMockSubResourcePatchFn(patchErr),
				},
				workload: &mock.Target{},
				condition: []condition.Condition{
					{},
				},
			},
			expected: errors.Wrap(patchErr, util.ErrUpdateStatus),
		},
	}
	for _, tt := range tests {
		err := util.EndReconcileWithPositiveCondition(tt.args.ctx, tt.args.r, tt.args.workload, tt.args.condition...)
		if tt.expected == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tt.expected.Error(), err.Error())
		}
	}
}

func TestAddLabels(t *testing.T) {
	basicLabels := map[string]string{
		"basic.label.key": "basic",
	}
	obj1 := new(unstructured.Unstructured)
	wantObj1 := new(unstructured.Unstructured)
	wantObj1.SetLabels(map[string]string{
		"basic.label.key": "basic",
		"newKey":          "newValue",
	})
	obj2 := new(unstructured.Unstructured)
	wantObj2 := new(unstructured.Unstructured)
	obj2.SetLabels(map[string]string{
		"key": "value",
	})
	wantObj2.SetLabels(map[string]string{
		"basic.label.key": "basic",
		"key":             "value",
		"newKey2":         "newValue2",
	})

	cases := map[string]struct {
		obj       *unstructured.Unstructured
		newLabels map[string]string
		want      *unstructured.Unstructured
	}{
		"add labels to workload without labels": {
			obj1,
			map[string]string{
				"newKey": "newValue",
			},
			wantObj1,
		},
		"add labels to workload with labels": {
			obj2,
			map[string]string{
				"newKey2": "newValue2",
			},
			wantObj2,
		},
	}

	for name, tc := range cases {
		t.Log("Running test case: " + name)
		obj := tc.obj
		wantObj := tc.want
		util.AddLabels(obj, basicLabels)
		util.AddLabels(obj, tc.newLabels)
		assert.Equal(t, wantObj.GetLabels(), obj.GetLabels())
	}
}

func TestMergeMapOverrideWithDst(t *testing.T) {
	const (
		basicKey   = "basicKey"
		dstKey     = "dstKey"
		srcKey     = "srcKey"
		basicValue = "basicValue"
		dstValue   = "dstValue"
		srcValue   = "srcValue"
	)
	basicDst := map[string]string{basicKey: basicValue}

	cases := map[string]struct {
		src  map[string]string
		dst  map[string]string
		want map[string]string
	}{
		"src is nil, dst is not nil": {
			src:  nil,
			dst:  map[string]string{dstKey: dstValue},
			want: map[string]string{basicKey: basicValue, dstKey: dstValue},
		},
		"src is not nil, dst is nil": {
			src:  map[string]string{srcKey: srcValue},
			dst:  nil,
			want: map[string]string{basicKey: basicValue, srcKey: srcValue},
		},
		"both nil": {
			src:  nil,
			dst:  nil,
			want: map[string]string{basicKey: basicValue},
		},
		"both not nil": {
			src:  map[string]string{srcKey: srcValue},
			dst:  map[string]string{dstKey: dstValue},
			want: map[string]string{basicKey: basicValue, srcKey: srcValue, dstKey: dstValue},
		},
	}
	for name, tc := range cases {
		t.Log("Running test case: " + name)
		result := util.MergeMapOverrideWithDst(tc.src, basicDst)
		result = util.MergeMapOverrideWithDst(result, tc.dst)
		assert.Equal(t, result, tc.want)
	}

}

func TestRawExtension2Map(t *testing.T) {
	r1 := runtime.RawExtension{
		Raw:    []byte(`{"a":{"c":"d"},"b":1}`),
		Object: nil,
	}
	exp1 := map[string]interface{}{
		"a": map[string]interface{}{
			"c": "d",
		},
		"b": float64(1),
	}
	got1, err := util.RawExtension2Map(&r1)
	assert.NoError(t, err)
	assert.Equal(t, exp1, got1)

	r2 := runtime.RawExtension{
		Raw: nil,
		Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"a": map[string]interface{}{
				"c": "d",
			},
			"b": float64(1),
		}},
	}
	got2, err := util.RawExtension2Map(&r2)
	assert.NoError(t, err)
	assert.Equal(t, exp1, got2)
}

func TestGenDefinitionNsFromCtx(t *testing.T) {
	type testcase struct {
		ctx    context.Context
		wantNs string
	}
	testcases := []testcase{
		{ctx: context.TODO(), wantNs: "vela-system"},
		{ctx: util.SetNamespaceInCtx(context.Background(), "vela-app"), wantNs: "vela-app"},
		{ctx: util.SetNamespaceInCtx(context.Background(), ""), wantNs: "default"},
	}
	for _, ts := range testcases {
		resNs := util.GetDefinitionNamespaceWithCtx(ts.ctx)
		assert.Equal(t, ts.wantNs, resNs)

	}
}

// TestGetDefinitionError is try to mock test when get an not existed definition  in namespaced scope cluster
// will get an error that tpye is not found
func TestGetDefinitionError(t *testing.T) {
	ctx := context.Background()
	ctx = util.SetNamespaceInCtx(ctx, "vela-app")

	errNotFound := apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "traitDefinition"}, "mock")
	errNeedNamespace := fmt.Errorf("an empty namespace may not be set when a resource name is provided")

	getFunc := func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		ns := key.Namespace
		if ns != "" {
			return errNotFound
		} else {
			return errNeedNamespace
		}
	}

	client := test.MockClient{MockGet: getFunc}
	td := new(v1beta1.TraitDefinition)
	got := util.GetDefinition(ctx, &client, td, "mock")
	assert.Equal(t, errNotFound, got)
}

// TestGetDefinitionWithClusterScope is try to test compatibility of GetDefinition,
// GetDefinition try to search definition in system-level namespace firstly,
// if not found will search in app namespace, still cannot find it, try to search definition without namespace
func TestGetDefinitionWithClusterScope(t *testing.T) {
	ctx := context.Background()
	ctx = util.SetNamespaceInCtx(ctx, "vela-app")
	// system-level definition
	sys := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sysDefinition",
			Namespace: "vela-system",
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}
	// app workload Definition
	app := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "appDefinition",
			Namespace: "vela-app",
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence",
			},
		},
	}
	// old cluster workload trait scope definition crd is cluster scope, the namespace field is empty
	noNs := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "noNsDefinition",
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence",
			},
		},
	}
	tdList := []v1beta1.TraitDefinition{app, sys, noNs}
	mockIndexer := map[string]v1beta1.TraitDefinition{}
	for i := 0; i < len(tdList); i++ {
		var key string
		if tdList[i].Namespace != "" {
			key = tdList[i].Namespace + "/" + tdList[i].Name
		} else {
			key = tdList[i].Name
		}
		mockIndexer[key] = tdList[i]
	}

	getFunc := func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		var namespacedName string
		if key.Namespace != "" {
			namespacedName = key.Namespace + "/" + key.Name
		} else {
			namespacedName = key.Name
		}
		td, ok := mockIndexer[namespacedName]
		if ok {
			obj, _ := obj.(*v1beta1.TraitDefinition)
			*obj = td
			return nil
		} else {
			return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "traitDefinition"}, namespacedName)
		}
	}

	type want struct {
		td  *v1beta1.TraitDefinition
		err error
	}
	testcases := map[string]struct {
		tdName string
		want   want
	}{
		"app namespace is first level": {
			tdName: "appDefinition",
			want: want{
				err: nil,
				td:  &app,
			},
		},
		"got sys namespace in system levle": {
			tdName: "sysDefinition",
			want: want{
				err: nil,
				td:  &sys,
			},
		},
		"old cluster traitdefinition crd is cluster scope": {
			tdName: "noNsDefinition",
			want: want{
				err: nil,
				td:  &noNs,
			},
		},
		"return err search not exsited definition": {
			tdName: "notExistedDefinition",
			want: want{
				err: apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "traitDefinition"}, "notExistedDefinition"),
				td:  new(v1beta1.TraitDefinition),
			},
		},
	}

	tclient := test.MockClient{MockGet: getFunc}

	for name, tc := range testcases {
		got := new(v1beta1.TraitDefinition)
		err := util.GetDefinition(ctx, &tclient, got, tc.tdName)
		t.Log(fmt.Sprint("Running test: ", name))

		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.td, got)
	}
}

func TestGetWorkloadDefinition(t *testing.T) {
	// Test common variables
	ctx := context.Background()
	ctx = util.SetNamespaceInCtx(ctx, "vela-app")

	// sys workload Definition
	sysWorkloadDefinition := v1beta1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
		Spec: v1beta1.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	// app workload Definition
	appWorkloadDefinition := v1beta1.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition.core.oam.dev",
			Namespace: "vela-app",
		},
		Spec: v1beta1.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	type fields struct {
		getFunc test.MockGetFn
	}
	type want struct {
		wld v1beta1.WorkloadDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{

		"app defintion will overlay system definition": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					o := obj.(*v1beta1.WorkloadDefinition)
					if key.Namespace == "vela-system" {
						*o = sysWorkloadDefinition
					} else {
						*o = appWorkloadDefinition
					}
					return nil
				},
			},
			want: want{
				wld: appWorkloadDefinition,
				err: nil,
			},
		},

		"return system definition when cannot find in app ns": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if key.Namespace == "vela-system" {
						o := obj.(*v1beta1.WorkloadDefinition)
						*o = sysWorkloadDefinition
						return nil
					}
					return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "workloadDefinition"}, key.Name)
				},
			},
			want: want{
				wld: sysWorkloadDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: tc.fields.getFunc,
		}
		got := new(v1beta1.WorkloadDefinition)
		err := util.GetDefinition(ctx, &tclient, got, "mockdefinition")
		t.Log(fmt.Sprint("Running test: ", name))

		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.wld, *got)
	}
}

func TestGetTraitDefinition(t *testing.T) {
	// Test common variables
	ctx := context.Background()
	ctx = util.SetNamespaceInCtx(ctx, "vela-app")

	// sys workload Definition
	sysTraitDefinition := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	// app workload Definition
	appTraitDefinition := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition.core.oam.dev",
			Namespace: "vela-app",
		},
		Spec: v1beta1.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	type fields struct {
		getFunc test.MockGetFn
	}
	type want struct {
		wld v1beta1.TraitDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"app defintion will overlay system definition": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					o := obj.(*v1beta1.TraitDefinition)
					if key.Namespace == "vela-system" {
						*o = sysTraitDefinition
					} else {
						*o = appTraitDefinition
					}
					return nil
				},
			},
			want: want{
				wld: appTraitDefinition,
				err: nil,
			},
		},

		"return system definition when cannot find in app ns": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if key.Namespace == "vela-system" {
						o := obj.(*v1beta1.TraitDefinition)
						*o = sysTraitDefinition
						return nil
					}
					return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "workloadDefinition"}, key.Name)
				},
			},
			want: want{
				wld: sysTraitDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: tc.fields.getFunc,
		}
		got := new(v1beta1.TraitDefinition)
		err := util.GetDefinition(ctx, &tclient, got, "mockdefinition")
		t.Log(fmt.Sprint("Running test: ", name))

		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.wld, *got)
	}
}

func TestGetDefinition(t *testing.T) {
	// Test common variables
	env := "env-namespace"

	// sys workload Definition
	sysTraitDefinition := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
	}

	// app workload Definition
	appTraitDefinition := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-app",
		},
	}

	// env workload Definition
	envTraitDefinition := v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: env,
		},
	}

	cli := test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		o := obj.(*v1beta1.TraitDefinition)
		switch key.Namespace {
		case "vela-system":
			*o = sysTraitDefinition
		case "vela-app":
			*o = appTraitDefinition
		case env:
			*o = envTraitDefinition
		default:
			return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "traitDefinition"}, key.Name)
		}
		return nil
	}}

	ctx := context.Background()
	ctx = util.SetNamespaceInCtx(ctx, "vela-app")
	appTd := new(v1beta1.TraitDefinition)
	err := util.GetDefinition(ctx, &cli, appTd, "mockTrait")
	assert.Equal(t, nil, err)
	assert.Equal(t, &appTraitDefinition, appTd)
}

func TestExtractRevisionNum(t *testing.T) {
	testcases := []struct {
		revName         string
		wantRevisionNum int
		delimiter       string
		hasError        bool
	}{{
		revName:         "myapp-v1",
		wantRevisionNum: 1,
		delimiter:       "-",
		hasError:        false,
	}, {
		revName:         "new-app-v2",
		wantRevisionNum: 2,
		delimiter:       "-",
		hasError:        false,
	}, {
		revName:         "v1-v10",
		wantRevisionNum: 10,
		delimiter:       "-",
		hasError:        false,
	}, {
		revName:         "v10-v1-v1",
		wantRevisionNum: 1,
		delimiter:       "-",
		hasError:        false,
	}, {
		revName:         "myapp-v1-v2",
		wantRevisionNum: 2,
		delimiter:       "-",
		hasError:        false,
	}, {
		revName:         "myapp-v1-vv",
		wantRevisionNum: 0,
		delimiter:       "-",
		hasError:        true,
	}, {
		revName:         "v1",
		wantRevisionNum: 0,
		delimiter:       "-",
		hasError:        true,
	}, {
		revName:         "myapp-a1",
		wantRevisionNum: 0,
		delimiter:       "-",
		hasError:        true,
	}, {
		revName:         "worker@v1",
		wantRevisionNum: 1,
		delimiter:       "@",
		hasError:        false,
	}, {
		revName:         "worke@10r@v1",
		wantRevisionNum: 1,
		delimiter:       "@",
		hasError:        false,
	}, {
		revName:         "webservice@a10",
		wantRevisionNum: 0,
		delimiter:       "@",
		hasError:        true,
	}}

	for _, tt := range testcases {
		revision, err := util.ExtractRevisionNum(tt.revName, tt.delimiter)
		hasError := err != nil
		assert.Equal(t, tt.wantRevisionNum, revision)
		assert.Equal(t, tt.hasError, hasError)
	}
}

func TestConvertDefinitionRevName(t *testing.T) {
	testcases := []struct {
		defName     string
		wantRevName string
		hasError    bool
	}{{
		defName:     "worker@v2",
		wantRevName: "worker-v2",
		hasError:    false,
	}, {
		defName:     "worker@v10",
		wantRevName: "worker-v10",
		hasError:    false,
	}, {
		defName:     "worker",
		wantRevName: "worker",
		hasError:    false,
	}, {
		defName:  "webservice@@v2",
		hasError: true,
	}, {
		defName:  "webservice@v10@v3",
		hasError: true,
	}, {
		defName:  "@v10",
		hasError: true,
	}}

	for _, tt := range testcases {
		revName, err := util.ConvertDefinitionRevName(tt.defName)
		assert.Equal(t, tt.hasError, err != nil)
		if !tt.hasError {
			assert.Equal(t, tt.wantRevName, revName)
		}
	}
}

func TestXDefinitionNamespaceInCtx(t *testing.T) {
	testcases := []struct {
		namespace         string
		expectedNamespace string
	}{{
		namespace:         "",
		expectedNamespace: oam.SystemDefinitionNamespace,
	}, {
		namespace:         oam.SystemDefinitionNamespace,
		expectedNamespace: oam.SystemDefinitionNamespace,
	}, {
		namespace:         "my-vela-system",
		expectedNamespace: "my-vela-system"},
	}

	ctx := context.Background()
	ns := util.GetXDefinitionNamespaceWithCtx(ctx)
	assert.Equal(t, oam.SystemDefinitionNamespace, ns)

	for _, tc := range testcases {
		ctx = util.SetXDefinitionNamespaceInCtx(ctx, tc.namespace)
		ns = util.GetXDefinitionNamespaceWithCtx(ctx)
		assert.Equal(t, tc.expectedNamespace, ns)
	}
}
