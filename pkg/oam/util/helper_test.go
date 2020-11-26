package util_test

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"os"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/mock"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
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
				getFunc: func(obj runtime.Object) error {
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
				getFunc: func(obj runtime.Object) error {
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
				getFunc: func(obj runtime.Object) error {
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

func TestFetchWorkloadTraitReference(t *testing.T) {

	t.Log("Setting up variables")
	log := ctrl.Log.WithName("ManualScalarTraitReconciler")
	noRefNameTrait := v1alpha2.ManualScalerTrait{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.ManualScalerTraitKind,
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
			WorkloadReference: v1alpha1.TypedReference{
				APIVersion: "apiversion",
				Kind:       "Kind",
			},
		},
	}
	// put the workload name back
	manualScalar := noRefNameTrait
	manualScalar.Spec.WorkloadReference.Name = "wokload-example"
	ctx := context.Background()
	wl := v1alpha2.ContainerizedWorkload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha2.SchemeGroupVersion.String(),
			Kind:       v1alpha2.ContainerizedWorkloadKind,
		},
	}
	uwl, _ := util.Object2Unstructured(wl)
	refErr := errors.New("no workload reference")
	workloadErr := fmt.Errorf("workload errr")

	type fields struct {
		trait   oam.Trait
		getFunc test.ObjectFn
	}
	type want struct {
		wl  *unstructured.Unstructured
		err error
	}
	cases := map[string]struct {
		fields fields
		want   want
	}{
		"FetchWorkload fail with mal-structured workloadRef": {
			fields: fields{
				trait: &noRefNameTrait,
			},
			want: want{
				wl:  nil,
				err: refErr,
			},
		},
		"FetchWorkload fails when getWorkload fails": {
			fields: fields{
				trait: &manualScalar,
				getFunc: func(obj runtime.Object) error {
					return workloadErr
				},
			},
			want: want{
				wl:  nil,
				err: workloadErr,
			},
		},
		"FetchWorkload succeeds when getWorkload succeeds": {
			fields: fields{
				trait: &manualScalar,
				getFunc: func(obj runtime.Object) error {
					o, _ := obj.(*unstructured.Unstructured)
					*o = *uwl
					return nil
				},
			},
			want: want{
				wl:  uwl,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.NewMockClient()
		tclient.MockGet = test.NewMockGetFn(nil, tc.fields.getFunc)
		gotWL, err := util.FetchWorkload(ctx, tclient, log, tc.fields.trait)
		t.Log(fmt.Sprint("Running test: ", name))
		if tc.want.err == nil {
			assert.NoError(t, err)
		} else {
			assert.Equal(t, tc.want.err.Error(), err.Error())
		}
		assert.Equal(t, tc.want.wl, gotWL)
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
			Reference: v1alpha2.DefinitionReference{
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
				getFunc: func(obj runtime.Object) error {
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
				getFunc: func(obj runtime.Object) error {
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

func TestTraitHelper(t *testing.T) {
	ctx := context.Background()
	namespace := "oamNS"
	traitDefinitionKind := "TraitDefinition"
	mockVerision := "core.oam.dev/v1alpha2"
	traitDefinitionName := "mocktraits.core.oam.dev"
	traitDefinitionRefName := "mocktraits.core.oam.dev"
	traitDefinitionWorkloadRefPath := "spec.workloadRef"

	mockTraitDefinition := v1alpha2.TraitDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       traitDefinitionKind,
			APIVersion: mockVerision,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      traitDefinitionName,
			Namespace: namespace,
		},
		Spec: v1alpha2.TraitDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: traitDefinitionRefName,
			},
			RevisionEnabled:    false,
			WorkloadRefPath:    traitDefinitionWorkloadRefPath,
			AppliesToWorkloads: nil,
		},
	}

	traitName := "ms-trait"

	mockTrait := v1alpha2.ManualScalerTrait{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      traitName,
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}

	unstructuredTrait, _ := util.Object2Unstructured(mockTrait)

	getErr := fmt.Errorf("get error")
	type fields struct {
		getFunc test.ObjectFn
	}
	type want struct {
		td  *v1alpha2.TraitDefinition
		err error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"FetchTraitDefinition fail when getTraitDefinition fails": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					return getErr
				},
			},
			want: want{
				td:  nil,
				err: getErr,
			},
		},

		"FetchTraitDefinition Success": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					o, _ := obj.(*v1alpha2.TraitDefinition)
					td := mockTraitDefinition
					*o = td
					return nil
				},
			},
			want: want{
				td:  &mockTraitDefinition,
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet: test.NewMockGetFn(nil, tc.fields.getFunc),
		}
		got, err := util.FetchTraitDefinition(ctx, &tclient, mock.NewMockDiscoveryMapper(), unstructuredTrait)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.td, got)
	}
}

func TestUtils(t *testing.T) {
	// Test common variables
	ctx := context.Background()
	namespace := "oamNS"
	workloadName := "oamWorkload"
	workloadKind := "ContainerizedWorkload"
	workloadAPIVersion := "core.oam.dev/v1"
	workloadDefinitionName := "containerizedworkloads.core.oam.dev"
	var workloadUID types.UID = "oamWorkloadUID"

	// workload CR
	workload := v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: workloadAPIVersion,
			Kind:       workloadKind,
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
			Reference: v1alpha2.DefinitionReference{
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
				getFunc: func(obj runtime.Object) error {
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
				getFunc: func(obj runtime.Object) error {
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

func TestChildResources(t *testing.T) {
	var workloadUID types.UID = "oamWorkloadUID"
	workloadDefinitionName := "containerizedworkloads.core.oam.dev"
	namespace := "oamNS"
	workloadName := "oamWorkload"
	workloadKind := "ContainerizedWorkload"
	workloadAPIVersion := "core.oam.dev/v1"
	// workload CR
	workload := v1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: workloadAPIVersion,
			Kind:       workloadKind,
		},
	}
	workload.SetUID(workloadUID)
	unstructuredWorkload, _ := util.Object2Unstructured(workload)
	ctx := context.Background()
	getErr := fmt.Errorf("get failed")
	// workload Definition
	workloadDefinition := v1alpha2.WorkloadDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: workloadDefinitionName,
		},
		Spec: v1alpha2.WorkloadDefinitionSpec{
			Reference: v1alpha2.DefinitionReference{
				Name: workloadDefinitionName,
			},
		},
	}

	log := ctrl.Log.WithName("ManualScalarTraitReconciler")
	crkl := []v1alpha2.ChildResourceKind{
		{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		{
			Kind:       "Service",
			APIVersion: "v1",
		},
	}
	// cdResource is the child deployment owned by the workload
	cdResource := unstructured.Unstructured{}
	cdResource.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind: util.KindDeployment,
			UID:  workloadUID,
		},
	})
	// cdResource is the child service owned by the workload
	cSResource := unstructured.Unstructured{}
	cSResource.SetOwnerReferences([]metav1.OwnerReference{
		{
			Kind: util.KindService,
			UID:  workloadUID,
		},
	})
	// oResource is not owned by the workload
	oResource := unstructured.Unstructured{}
	oResource.SetOwnerReferences([]metav1.OwnerReference{
		{
			UID: "NotWorkloadUID",
		},
	})
	var nilListFunc test.ObjectFn = func(o runtime.Object) error {
		u := &unstructured.Unstructured{}
		l := o.(*unstructured.UnstructuredList)
		l.Items = []unstructured.Unstructured{*u}
		return nil
	}
	type fields struct {
		getFunc  test.ObjectFn
		listFunc test.ObjectFn
	}
	type want struct {
		crks []*unstructured.Unstructured
		err  error
	}

	cases := map[string]struct {
		fields fields
		want   want
	}{
		"FetchWorkloadChildResources fail when getWorkloadDefinition fails": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					return getErr
				},
				listFunc: nilListFunc,
			},
			want: want{
				crks: nil,
				err:  getErr,
			},
		},
		"FetchWorkloadChildResources return nothing when the workloadDefinition doesn't have child list": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					o, _ := obj.(*v1alpha2.WorkloadDefinition)
					*o = workloadDefinition
					return nil
				},
				listFunc: nilListFunc,
			},
			want: want{
				crks: nil,
				err:  nil,
			},
		},
		"FetchWorkloadChildResources Success": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					o, _ := obj.(*v1alpha2.WorkloadDefinition)
					w := workloadDefinition
					w.Spec.ChildResourceKinds = crkl
					*o = w
					return nil
				},
				listFunc: func(o runtime.Object) error {
					l := o.(*unstructured.UnstructuredList)
					switch l.GetKind() {
					case util.KindDeployment:
						l.Items = append(l.Items, cdResource)
					case util.KindService:
						l.Items = append(l.Items, cSResource)
					default:
						return getErr
					}
					return nil
				},
			},
			want: want{
				crks: []*unstructured.Unstructured{
					&cdResource, &cSResource,
				},
				err: nil,
			},
		},
		"FetchWorkloadChildResources with many resources only pick the child one": {
			fields: fields{
				getFunc: func(obj runtime.Object) error {
					o, _ := obj.(*v1alpha2.WorkloadDefinition)
					w := workloadDefinition
					w.Spec.ChildResourceKinds = crkl
					*o = w
					return nil
				},
				listFunc: func(o runtime.Object) error {
					l := o.(*unstructured.UnstructuredList)
					l.Items = []unstructured.Unstructured{oResource, oResource, oResource, oResource,
						oResource, oResource, oResource}
					if l.GetKind() == util.KindDeployment {
						l.Items = append(l.Items, cdResource)
					} else if l.GetKind() != util.KindService {
						return getErr
					}
					return nil
				},
			},
			want: want{
				crks: []*unstructured.Unstructured{
					&cdResource,
				},
				err: nil,
			},
		},
	}
	for name, tc := range cases {
		tclient := test.MockClient{
			MockGet:  test.NewMockGetFn(nil, tc.fields.getFunc),
			MockList: test.NewMockListFn(nil, tc.fields.listFunc),
		}
		got, err := util.FetchWorkloadChildResources(ctx, log, &tclient, mock.NewMockDiscoveryMapper(), unstructuredWorkload)
		t.Log(fmt.Sprint("Running test: ", name))
		assert.Equal(t, tc.want.err, err)
		assert.Equal(t, tc.want.crks, got)
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
	gvk, err := util.GetGVKFromDefinition(mapper, v1alpha2.DefinitionReference{Name: "abcs.example.com"})
	assert.NoError(t, err)
	assert.Equal(t, schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v1",
		Kind:    "Abc",
	}, gvk)

	gvk, err = util.GetGVKFromDefinition(mapper, v1alpha2.DefinitionReference{Name: "abcs.example.com", Version: "v2"})
	assert.NoError(t, err)
	assert.Equal(t, schema.GroupVersionKind{
		Group:   "example.com",
		Version: "v2",
		Kind:    "Abc",
	}, gvk)
}

func TestGenTraitName(t *testing.T) {
	mts := v1alpha2.ManualScalerTrait{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "sample-manualscaler-trait",
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}
	trait := v1alpha2.ManualScalerTrait{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "extend.oam.dev/v1alpha2",
			Kind:       "ManualScalerTrait",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "sample-manualscaler-trait",
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}
	traitTemplate := &v1alpha2.ComponentTrait{
		Trait: runtime.RawExtension{
			Object: &trait,
		},
	}

	tests := []struct {
		name           string
		template       *v1alpha2.ComponentTrait
		definitionName string
		exp            string
	}{
		{
			name:           "simple",
			template:       &v1alpha2.ComponentTrait{},
			definitionName: "",
			exp:            "simple-trait-67b8949f8d",
		},
		{
			name: "simple",
			template: &v1alpha2.ComponentTrait{
				Trait: runtime.RawExtension{
					Object: &mts,
				},
			},
			definitionName: "",
			exp:            "simple-trait-5ddc8b7556",
		},
		{
			name:           "simple-definition",
			template:       traitTemplate,
			definitionName: "autoscale",
			exp:            "simple-definition-autoscale-" + util.ComputeHash(traitTemplate),
		},
	}
	for _, test := range tests {
		got := util.GenTraitName(test.name, test.template, test.definitionName)
		t.Log(fmt.Sprint("Running test: ", test.name))
		assert.Equal(t, test.exp, got)
	}
}

func TestComputeHash(t *testing.T) {
	mts := v1alpha2.ManualScalerTrait{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "sample-manualscaler-trait",
		},
		Spec: v1alpha2.ManualScalerTraitSpec{
			ReplicaCount: 3,
		},
	}

	test := []struct {
		name     string
		template *v1alpha2.ComponentTrait
		exp      string
	}{
		{
			name:     "simple",
			template: &v1alpha2.ComponentTrait{},
			exp:      "67b8949f8d",
		},
		{
			name: "simple",
			template: &v1alpha2.ComponentTrait{
				Trait: runtime.RawExtension{
					Object: &mts,
				},
			},
			exp: "5ddc8b7556",
		},
	}
	for _, test := range test {
		got := util.ComputeHash(test.template)

		t.Log(fmt.Sprint("Running test: ", got))
		assert.Equal(t, test.exp, got)
	}
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

func TestPatchCondition(t *testing.T) {
	type args struct {
		ctx       context.Context
		r         client.StatusClient
		workload  util.ConditionedObject
		condition []v1alpha1.Condition
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
					MockStatusPatch: test.NewMockStatusPatchFn(nil),
				},
				workload: &fake.Target{},
				condition: []v1alpha1.Condition{
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
					MockStatusPatch: test.NewMockStatusPatchFn(patchErr),
				},
				workload: &fake.Target{},
				condition: []v1alpha1.Condition{
					{},
				},
			},
			expected: errors.Wrap(patchErr, util.ErrUpdateStatus),
		},
	}
	for _, tt := range tests {
		err := util.PatchCondition(tt.args.ctx, tt.args.r, tt.args.workload, tt.args.condition...)
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
			LatestRevision: &v1alpha2.Revision{Name: revisionName2, Revision: 2},
		},
	}

	client := &test.MockClient{MockGet: func(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
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
	var u unstructured.Unstructured
	util.PassLabelAndAnnotation(ac, &u)
	got := u.GetLabels()
	want := labels
	assert.Equal(t, want, got)
	gotAnnotation := u.GetAnnotations()
	wantAnnotation := annotation
	assert.Equal(t, wantAnnotation, gotAnnotation)
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
		"key1": "exist value1",
		"key2": "value2",
		"key3": "value3",
	}
	assert.Equal(t, wantAnnotation, gotAnnotation)

	gotLabels := u.GetLabels()
	wantLabels := map[string]string{
		"core.oam.dev/ns":          "kube-system",
		"core.oam.dev/kube-native": "deployment",
		"core.oam.dev/controller":  "oam-kubernetes-runtime",
	}
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
		Spec: v1alpha2.TraitDefinitionSpec{Reference: v1alpha2.DefinitionReference{Name: "dummy"}},
	}, util.GetDummyTraitDefinition(u))
	assert.Equal(t, &v1alpha2.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{Kind: v1alpha2.WorkloadDefinitionKind, APIVersion: v1alpha2.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{Name: "dummy", Annotations: map[string]string{
			"apiVersion": u.GetAPIVersion(),
			"kind":       u.GetKind(),
			"name":       u.GetName(),
		}},
		Spec: v1alpha2.WorkloadDefinitionSpec{Reference: v1alpha2.DefinitionReference{Name: "dummy"}},
	}, util.GetDummyWorkloadDefinition(u))
}

func TestNamespacedDefinition(t *testing.T) {
	ns := "namespaced"
	n := "definition"
	_ = os.Setenv(util.DefinitionNamespaceEnv, ns)
	nn := util.GenNamespacedDefinitionName(n)
	assert.Equal(t, nn.Namespace, ns)
	assert.Equal(t, nn.Name, n)
}
