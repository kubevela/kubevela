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
	"encoding/json"
	"fmt"
	"hash/adler32"
	"reflect"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/condition"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/mock"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestLocateParentAppConfig(t *testing.T) {
	ctx := context.Background()
	const namespace = "oamNS"
	acKind := reflect.TypeOf(v1alpha2.ApplicationConfiguration{}).Name()
	mockVersion := "core.oam.dev/v1alpha2"
	acName := "mockAC"

	mockAC := v1alpha2.ApplicationConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       acKind,
			APIVersion: mockVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      acName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ApplicationConfigurationSpec{
			Components: nil,
		},
	}

	mockOwnerRef := metav1.OwnerReference{
		APIVersion: mockVersion,
		Kind:       acKind,
		Name:       acName,
	}

	cmpKind := "Component"
	cmpName := "mockComponent"

	// use Component as mock oam.Object
	mockComp := v1alpha2.Component{
		TypeMeta: metav1.TypeMeta{
			Kind:       cmpKind,
			APIVersion: mockVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmpName,
			Namespace:       namespace,
			OwnerReferences: []metav1.OwnerReference{mockOwnerRef},
		},
		Spec: v1alpha2.ComponentSpec{
			Workload: runtime.RawExtension{
				Raw:    nil,
				Object: nil,
			},
			Parameters: nil,
		},
	}

	mockCompWithEmptyOwnerRef := mockComp
	mockCompWithEmptyOwnerRef.ObjectMeta.OwnerReferences = nil

	getErr := fmt.Errorf("get error")
	type fields struct {
		getFunc test.ObjectFn
		oamObj  oam.Object
	}
	type want struct {
		ac  oam.Object
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"LocateParentAppConfig fail when getAppConfig fails": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					return getErr
				},
				oamObj: &mockComp,
			},
			want: want{
				ac:  nil,
				err: getErr,
			},
		},

		"LocateParentAppConfig fail when no ApplicationConfiguration in OwnerReferences": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					return getErr
				},
				oamObj: &mockCompWithEmptyOwnerRef,
			},
			want: want{
				ac:  nil,
				err: errors.Errorf(util.ErrLocateAppConfig),
			},
		},
		"LocateParentAppConfig success": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					o, _ := obj.(*v1alpha2.ApplicationConfiguration)
					ac := mockAC
					*o = ac
					return nil
				},
				oamObj: &mockComp,
			},
			want: want{
				ac:  &mockAC,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
		}
		got, err := util.LocateParentAppConfig(ctx, &tclient, tc.fields.oamObj)
		t.Log(fmt.Sprint("Running test: ", name))
		if tc.want.err == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tc.want.err.Error(), err.Error())
		}
		if tc.want.ac == nil {
			assert.Equal(t, tc.want.ac, nil)
		} else {
			assert.Equal(t, tc.want.ac, got)
		}
	}
}

func TestScopeRelatedUtils(t *testing.T) {

	ctx := context.Background()
	namespace := "oamNS"
	scopeDefinitionKind := "ScopeDefinition"
	mockVerision := "core.oam.dev/v1alpha2"
	scopeDefinitionName := "mockscopes.core.oam.dev"
	scopeDefinitionRefName := "mockscopes.core.oam.dev"
	scopeDefinitionWorkloadRefsPath := "spec.workloadRefs"

	mockScopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeDefinitionName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: scopeDefinitionRefName,
			},
			WorkloadRefsPath:      scopeDefinitionWorkloadRefsPath,
			AllowComponentOverlap: false,
		},
	}

	scopeKind := "HealthScope"
	scopeName := "HealthScope"

	mockScope := v1alpha2.HealthScope{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeName,
			Namespace: namespace,
		},
		Spec: v1alpha2.HealthScopeSpec{
			ProbeTimeout:       nil,
			ProbeInterval:      nil,
			WorkloadReferences: nil,
		},
	}

	unstructuredScope, _ := util.Object2Unstructured(mockScope)

	getErr := fmt.Errorf("get error")

	type fields struct {
		getFunc test.ObjectFn
	}
	type want struct {
		spd *v1alpha2.ScopeDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"FetchScopeDefinition fail when getScopeDefinition fails": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					return getErr
				},
			},
			want: want{
				spd: nil,
				err: getErr,
			},
		},

		"FetchScopeDefinition Success": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					o, _ := obj.(*v1alpha2.ScopeDefinition)
					sd := mockScopeDefinition
					*o = sd
					return nil
				},
			},
			want: want{
				spd: &mockScopeDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
		}
		got, err := util.FetchScopeDefinition(ctx, &tclient, mock.NewMockDiscoveryMapper(), unstructuredScope)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.spd, got)
	}
}

func TestUtils(t *testing.T) {
	// Test common variables
	ctx := context.Background()
	namespace := "oamNS"
	workloadName := "oamWorkload"
	imageV1 := "wordpress:4.6.1-apache"
	workloadDefinitionName := "deployments.apps"
	var workloadUID types.UID = "oamWorkloadUID"

	workload := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      workloadName,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "wordpress",
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "wordpress",
							Image: imageV1,
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "wordpress"}},
			},
		},
	}
	workload.SetUID(workloadUID)
	unstructuredWorkload, _ := util.Object2Unstructured(workload)
	// workload Definition
	workloadDefinition := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: workloadDefinitionName,
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: workloadDefinitionName,
			},
		},
	}

	getErr := fmt.Errorf("get failed")

	type fields struct {
		getFunc test.ObjectFn
	}
	type want struct {
		wld *v1alpha2.WorkloadDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"FetchWorkloadDefinition fail when getWorkloadDefinition fails": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					return getErr
				},
			},
			want: want{
				wld: nil,
				err: getErr,
			},
		},

		"FetchWorkloadDefinition Success": {
			fields: fields{
				getFunc: func(obj client.Object) error {
					o, _ := obj.(*v1alpha2.WorkloadDefinition)
					w := workloadDefinition
					*o = w
					return nil
				},
			},
			want: want{
				wld: &workloadDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
		}
		got, err := util.FetchWorkloadDefinition(ctx, &tclient, mock.NewMockDiscoveryMapper(), unstructuredWorkload)
		t.Log(fmt.Sprint("Running test: ", name))

		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.wld, got)
	}
}

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
		"extended resource": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "extend.oam.dev/v1alpha2",
				"kind":       "SimpleRolloutTrait",
			}},
			resource: "simplerollouttraits",
			exp:      "simplerollouttraits.extend.oam.dev",
		},
		"trait": {
			u: &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "extend.oam.dev/v1alpha2",
				"kind":       "SimpleRolloutTrait",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "rollout",
					},
				},
			}},
			typeLabel: oam.TraitTypeLabel,
			exp:       "rollout",
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
		mapper := mock.NewMockDiscoveryMapper()
		mapper.MockRESTMapping = mock.NewMockRESTMapping(ti.resource)
		got, err := util.GetDefinitionName(mapper, ti.u, ti.typeLabel)
		assert.NoError(t, err)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, ti.exp, got)
	}
}

func TestGetGVKFromDef(t *testing.T) {
	mapper := mock.NewMockDiscoveryMapper()
	mapper.MockKindsFor = mock.NewMockKindsFor("Abc", "v1", "v2")
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
	mapper := mock.NewMockDiscoveryMapper()

	mapper.MockRESTMapping = mock.NewMockRESTMapping("clonesets")
	ref, err := util.ConvertWorkloadGVK2Definition(mapper, common.WorkloadGVK{APIVersion: "apps.kruise.io/v1alpha1",
		Kind: "CloneSet"})
	assert.NoError(t, err)
	assert.Equal(t, common.DefinitionReference{
		Name:    "clonesets.apps.kruise.io",
		Version: "v1alpha1",
	}, ref)

	mapper.MockRESTMapping = mock.NewMockRESTMapping("deployments")
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

func TestComponentHelper(t *testing.T) {
	ctx := context.Background()
	type Case struct {
		caseName           string
		acc                v1alpha2.ApplicationConfigurationComponent
		expectComponent    *v1alpha2.Component
		expectRevisionName string
		expectErrorMatcher bool
	}

	namespace := "ns"
	componentName := "newcomponent"
	invalidComponentName := "invalidComponent"
	revisionName := "newcomponent-aa1111"
	revisionName2 := "newcomponent-bb1111"
	unpackErrorRevisionName := "unpackErrorRevision"
	errComponentNotFound := errors.New("component not found")

	componnet1 := v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: namespace,
		},
		Spec:   v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{}}},
		Status: v1alpha2.ComponentStatus{},
	}

	component2 := v1alpha2.Component{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ComponentSpec{Workload: runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"apiVersion": "New",
			},
		}}}},
		Status: v1alpha2.ComponentStatus{
			LatestRevision: &common.Revision{Name: revisionName2, Revision: 2},
		},
	}

	client := &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		if o, ok := obj.(*v1alpha2.Component); ok {
			switch key.Name {
			case componentName:
				*o = component2
			case invalidComponentName:
				return errComponentNotFound
			default:
				return nil
			}
		}
		if o, ok := obj.(*appsv1.ControllerRevision); ok {
			switch key.Name {
			case revisionName:
				*o = appsv1.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{Name: revisionName, Namespace: namespace},
					Data:       runtime.RawExtension{Object: &componnet1},
					Revision:   1,
				}
			case unpackErrorRevisionName:
				*o = appsv1.ControllerRevision{
					ObjectMeta: metav1.ObjectMeta{Name: unpackErrorRevisionName, Namespace: namespace},
					Data:       runtime.RawExtension{},
					Revision:   1,
				}
			default:
				return nil
			}
		}
		return nil

	}}
	testCases := []Case{
		{
			caseName:           "get component by revisionName",
			acc:                v1alpha2.ApplicationConfigurationComponent{RevisionName: revisionName},
			expectComponent:    &componnet1,
			expectRevisionName: revisionName,
			expectErrorMatcher: true,
		},
		{
			caseName:           "get component by componentName",
			acc:                v1alpha2.ApplicationConfigurationComponent{ComponentName: componentName},
			expectComponent:    &component2,
			expectRevisionName: revisionName2,
			expectErrorMatcher: true,
		},
		{
			caseName:           "not found error occurs when get by revisionName",
			acc:                v1alpha2.ApplicationConfigurationComponent{RevisionName: "invalidRevisionName"},
			expectComponent:    nil,
			expectRevisionName: "",
			expectErrorMatcher: false,
		},
		{
			caseName:           "unpack revison data error occurs when get by revisionName",
			acc:                v1alpha2.ApplicationConfigurationComponent{RevisionName: unpackErrorRevisionName},
			expectComponent:    nil,
			expectRevisionName: "",
			expectErrorMatcher: false,
		},
		{
			caseName:           "error occurs when get by componentName",
			acc:                v1alpha2.ApplicationConfigurationComponent{ComponentName: invalidComponentName},
			expectComponent:    nil,
			expectRevisionName: "",
			expectErrorMatcher: false,
		},
	}

	for _, tc := range testCases {
		t.Log("Running:" + tc.caseName)
		c, r, err := util.GetComponent(ctx, client, tc.acc, namespace)
		assert.Equal(t, tc.expectComponent, c)
		assert.Equal(t, tc.expectRevisionName, r)
		if tc.expectErrorMatcher {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestUnpackRevisionData(t *testing.T) {
	comp1 := v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}
	comp1Raw, _ := json.Marshal(comp1)
	tests := map[string]struct {
		rev     *appsv1.ControllerRevision
		expComp *v1alpha2.Component
		expErr  error
		reason  string
	}{
		"controllerRevision with Component Obj": {
			rev:     &appsv1.ControllerRevision{Data: runtime.RawExtension{Object: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}}}},
			expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
			reason:  "controllerRevision should align with component object",
		},
		"controllerRevision with Unknown Obj": {
			rev:    &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Object: &runtime.Unknown{Raw: comp1Raw}}},
			reason: "controllerRevision must be decode into component object",
			expErr: fmt.Errorf("invalid type of revision rev1, type should not be *runtime.Unknown"),
		},
		"unmarshal with component data": {
			rev:     &appsv1.ControllerRevision{ObjectMeta: metav1.ObjectMeta{Name: "rev1"}, Data: runtime.RawExtension{Raw: comp1Raw}},
			reason:  "controllerRevision should unmarshal data and align with component object",
			expComp: &v1alpha2.Component{ObjectMeta: metav1.ObjectMeta{Name: "comp1"}},
		},
	}
	for name, ti := range tests {
		t.Log("Running: " + name)
		comp, err := util.UnpackRevisionData(ti.rev)
		if ti.expErr != nil {
			assert.Equal(t, ti.expErr, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, ti.expComp, comp)
		}
	}
}

func TestPassThroughObjMeta(t *testing.T) {
	ac := &v1alpha2.ApplicationConfiguration{}
	labels := map[string]string{
		"core.oam.dev/ns":         "oam-system",
		"core.oam.dev/controller": "oam-kubernetes-runtime",
	}
	annotation := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	ac.SetLabels(labels)
	ac.SetAnnotations(annotation)
	t.Log("workload and trait have no labels and annotation")
	// test initial pass
	var u unstructured.Unstructured
	util.PassLabelAndAnnotation(ac, &u)
	got := u.GetLabels()
	want := labels
	assert.Equal(t, want, got)
	gotAnnotation := u.GetAnnotations()
	wantAnnotation := annotation
	assert.Equal(t, wantAnnotation, gotAnnotation)
	// test overlapping keys
	t.Log("workload and trait contains overlapping keys")
	existAnnotation := map[string]string{
		"key1": "exist value1",
		"key3": "value3",
	}
	existLabels := map[string]string{
		"core.oam.dev/ns":          "kube-system",
		"core.oam.dev/kube-native": "deployment",
	}
	u.SetLabels(existLabels)
	u.SetAnnotations(existAnnotation)
	util.PassLabelAndAnnotation(ac, &u)
	gotAnnotation = u.GetAnnotations()
	wantAnnotation = map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	assert.Equal(t, wantAnnotation, gotAnnotation)
	gotLabels := u.GetLabels()
	wantLabels := map[string]string{
		"core.oam.dev/ns":          "oam-system",
		"core.oam.dev/kube-native": "deployment",
		"core.oam.dev/controller":  "oam-kubernetes-runtime",
	}
	assert.Equal(t, wantLabels, gotLabels)

	// test removing annotation
	t.Log("removing parent key doesn't remove child's")
	util.RemoveAnnotations(ac, []string{"key1", "key2"})
	assert.Equal(t, len(ac.GetAnnotations()), 0)
	util.PassLabelAndAnnotation(ac, &u)
	gotAnnotation = u.GetAnnotations()
	assert.Equal(t, wantAnnotation, gotAnnotation)
	gotLabels = u.GetLabels()
	assert.Equal(t, wantLabels, gotLabels)
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

func TestGetDummy(t *testing.T) {
	var u = &unstructured.Unstructured{}
	u.SetKind("Testkind")
	u.SetAPIVersion("test.api/v1")
	u.SetName("testdummy")
	assert.Equal(t, &v1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{Kind: v1alpha2.TraitDefinitionKind, APIVersion: v1alpha2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "dummy", Annotations: map[string]string{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"name":       u.GetName(),
		}},
		Spec: v1alpha2.TraitDefinitionSpec{Reference: common.DefinitionReference{Name: "dummy"}},
	}, util.GetDummyTraitDefinition(u))
	assert.Equal(t, &v1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{Kind: v1alpha2.WorkloadDefinitionKind, APIVersion: v1alpha2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "dummy", Annotations: map[string]string{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"name":       u.GetName(),
		}},
		Spec: v1alpha2.WorkloadDefinitionSpec{Reference: common.DefinitionReference{Name: "dummy"}},
	}, util.GetDummyWorkloadDefinition(u))
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
	td := new(v1alpha2.TraitDefinition)
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
	sys := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sysDefinition",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}
	// app workload Definition
	app := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "appDefinition",
			Namespace: "vela-app",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence",
			},
		},
	}
	// old cluster workload trait scope definition crd is cluster scope, the namespace field is empty
	noNs := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "noNsDefinition",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence",
			},
		},
	}
	tdList := []v1alpha2.TraitDefinition{app, sys, noNs}
	mockIndexer := map[string]v1alpha2.TraitDefinition{}
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
			obj, _ := obj.(*v1alpha2.TraitDefinition)
			*obj = td
			return nil
		} else {
			return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "traitDefinition"}, namespacedName)
		}
	}

	type want struct {
		td  *v1alpha2.TraitDefinition
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
				td:  new(v1alpha2.TraitDefinition),
			},
		},
	}

	tclient := test.MockClient{MockGet: getFunc}

	for name, tc := range testcases {
		got := new(v1alpha2.TraitDefinition)
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
	sysWorkloadDefinition := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	// app workload Definition
	appWorkloadDefinition := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition.core.oam.dev",
			Namespace: "vela-app",
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	type fields struct {
		getFunc test.MockGetFn
	}
	type want struct {
		wld v1alpha2.WorkloadDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{

		"app defintion will overlay system definition": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					o := obj.(*v1alpha2.WorkloadDefinition)
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
						o := obj.(*v1alpha2.WorkloadDefinition)
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
		got := new(v1alpha2.WorkloadDefinition)
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
	sysTraitDefinition := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	// app workload Definition
	appTraitDefinition := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition.core.oam.dev",
			Namespace: "vela-app",
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "definitionrefrence.core.oam.dev",
			},
		},
	}

	type fields struct {
		getFunc test.MockGetFn
	}
	type want struct {
		wld v1alpha2.TraitDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"app defintion will overlay system definition": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					o := obj.(*v1alpha2.TraitDefinition)
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
						o := obj.(*v1alpha2.TraitDefinition)
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
		got := new(v1alpha2.TraitDefinition)
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
	sysTraitDefinition := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-system",
		},
	}

	// app workload Definition
	appTraitDefinition := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: "vela-app",
		},
	}

	// env workload Definition
	envTraitDefinition := v1alpha2.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mockdefinition",
			Namespace: env,
		},
	}

	cli := test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
		o := obj.(*v1alpha2.TraitDefinition)
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
	appTd := new(v1alpha2.TraitDefinition)
	err := util.GetDefinition(ctx, &cli, appTd, "mockTrait")
	assert.Equal(t, nil, err)
	assert.Equal(t, &appTraitDefinition, appTd)
}

func TestGetScopeDefinition(t *testing.T) {
	ctx := context.Background()
	namespace := "vela-app"
	ctx = util.SetNamespaceInCtx(ctx, namespace)
	scopeDefinitionKind := "ScopeDefinition"
	mockVerision := "core.oam.dev/v1alpha2"
	scopeDefinitionName := "mockscopes.core.oam.dev"
	scopeDefinitionRefName := "mockscopes.core.oam.dev"
	scopeDefinitionWorkloadRefsPath := "spec.workloadRefs"

	sysScopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeDefinitionName,
			Namespace: "vela-system",
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: scopeDefinitionRefName,
			},
			WorkloadRefsPath:      scopeDefinitionWorkloadRefsPath,
			AllowComponentOverlap: false,
		},
	}

	appScopeDefinition := v1alpha2.ScopeDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       scopeDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      scopeDefinitionName,
			Namespace: namespace,
		},
		Spec: v1alpha2.ScopeDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: scopeDefinitionRefName,
			},
			WorkloadRefsPath:      scopeDefinitionWorkloadRefsPath,
			AllowComponentOverlap: false,
		},
	}
	type fields struct {
		getFunc test.MockGetFn
	}
	type want struct {
		spd *v1alpha2.ScopeDefinition
		err error
	}
	cases := map[string]struct {
		fields fields
		want   want
	}{
		"app defintion will overlay system definition": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					o := obj.(*v1alpha2.ScopeDefinition)
					if key.Namespace == "vela-system" {
						*o = sysScopeDefinition
					} else {
						*o = appScopeDefinition
					}
					return nil
				},
			},
			want: want{
				spd: &appScopeDefinition,
				err: nil,
			},
		},

		"return system definition when cannot find in app ns": {
			fields: fields{
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if key.Namespace == "vela-system" {
						o := obj.(*v1alpha2.ScopeDefinition)
						*o = sysScopeDefinition
						return nil
					}
					return apierrors.NewNotFound(schema.GroupResource{Group: "core.oma.dev", Resource: "scopeDefinition"}, key.Name)
				},
			},
			want: want{
				spd: &sysScopeDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: tc.fields.getFunc,
		}
		got := new(v1alpha2.ScopeDefinition)
		err := util.GetDefinition(ctx, &tclient, got, "mockdefinition")
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.spd, got)
	}
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
