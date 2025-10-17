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

package appfile

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

type fakeRESTMapper struct {
	meta.RESTMapper
}

func (f fakeRESTMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	if resource.Resource == "deployments" {
		return schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, nil
	}
	return schema.GroupVersionKind{}, errors.New("no mapping for KindFor")
}

func (f fakeRESTMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	if resource.Resource == "deployments" {
		return []schema.GroupVersionKind{{Group: "apps", Version: "v1", Kind: "Deployment"}}, nil
	}
	return nil, errors.New("no mapping for KindsFor")
}

func TestLoadComponentTemplate(t *testing.T) {
	cueTemplate := `
      context: {
         name: "test"
      }
      output: {
      	apiVersion: "apps/v1"
      	kind:       "Deployment"
      	spec: {
      		selector: matchLabels: {
      			"app.oam.dev/component": context.name
      		}

      		template: {
      			metadata: labels: {
      				"app.oam.dev/component": context.name
      			}

      			spec: {
      				containers: [{
      					name:  context.name
      					image: parameter.image

      					if parameter["cmd"] != _|_ {
      						command: parameter.cmd
      					}
      				}]
      			}
      		}

      		selector:
      			matchLabels:
      				"app.oam.dev/component": context.name
      	}
      }

      parameter: {
      	// +usage=Which image would you like to use for your service
      	// +short=i
      	image: string

      	cmd?: [...string]
      }
      `

	var componentDefintion = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  namespace: default
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
` + cueTemplate

	// Create mock client
	tclient := test.MockClient{
		MockGet: func(ctx context.Context, key ktypes.NamespacedName, obj client.Object) error {
			switch o := obj.(type) {
			case *v1beta1.ComponentDefinition:
				cd, err := oamutil.UnMarshalStringToComponentDefinition(componentDefintion)
				if err != nil {
					return err
				}
				*o = *cd
			}
			return nil
		},
	}
	var annotations = make(map[string]string)
	temp, err := LoadTemplate(context.TODO(), &tclient, "worker", types.TypeComponentDefinition, annotations)

	if err != nil {
		t.Error(err)
		return
	}
	inst := cuecontext.New().CompileString(temp.TemplateStr)
	instDest := cuecontext.New().CompileString(cueTemplate)
	s1, _ := inst.Value().String()
	s2, _ := instDest.Value().String()
	if s1 != s2 {
		t.Errorf("parsered template is not correct")
	}
}

func TestLoadTraitTemplate(t *testing.T) {
	cueTemplate := `
        parameter: {
        	domain: string
        	http: [string]: int
        }
        context: {
        	name: "test"
        }
        // trait template can have multiple outputs in one trait
        outputs: service: {
        	apiVersion: "v1"
        	kind:       "Service"
        	metadata:
        		name: context.name
        	spec: {
        		selector:
        			"app.oam.dev/component": context.name
        		ports: [
        			for k, v in parameter.http {
        				port:       v
        				targetPort: v
        			},
        		]
        	}
        }

        outputs: ingress: {
        	apiVersion: "networking.k8s.io/v1"
        	kind:       "Ingress"
        	metadata:
        		name: context.name
        	spec: {
        		rules: [{
        			host: parameter.domain
        			http: {
        				paths: [
        					for k, v in parameter.http {
        						path: k
        						pathType: "Prefix"
        						backend: {
        							service: {
        								name: context.name
        								port: {
        									number: v
        								}
        							}
        						}
        					},
        				]
        			}
        		}]
        	}
        }
      `

	var traitDefintion = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Configures K8s ingress and service to enable web traffic for your service.
    Please use route trait in cap center for advanced usage."
  name: ingress
  namespace: default
spec:
  status:
    customStatus: |-
      if len(context.outputs.ingress.status.loadBalancer.ingress) > 0 {
      	message: "Visiting URL: " + context.outputs.ingress.spec.rules[0].host + ", IP: " + context.outputs.ingress.status.loadBalancer.ingress[0].ip
      }
      if len(context.outputs.ingress.status.loadBalancer.ingress) == 0 {
      	message: "No loadBalancer found, visiting by using 'vela port-forward " + context.appName + " --route'\n"
      }
    healthPolicy: |
      isHealth: len(context.outputs.service.spec.clusterIP) > 0
  appliesToWorkloads:
    - deployments.apps
  schematic:
    cue:
      template: |
` + cueTemplate

	// Create mock client
	tclient := test.MockClient{
		MockGet: func(ctx context.Context, key ktypes.NamespacedName, obj client.Object) error {
			switch o := obj.(type) {
			case *v1beta1.TraitDefinition:
				wd, err := oamutil.UnMarshalStringToTraitDefinition(traitDefintion)
				if err != nil {
					return err
				}
				*o = *wd
			}
			return nil
		},
	}
	var annotations = make(map[string]string)
	temp, err := LoadTemplate(context.TODO(), &tclient, "ingress", types.TypeTrait, annotations)

	if err != nil {
		t.Error(err)
		return
	}
	inst := cuecontext.New().CompileString(temp.TemplateStr)
	instDest := cuecontext.New().CompileString(cueTemplate)
	s1, _ := inst.Value().String()
	s2, _ := instDest.Value().String()
	if s1 != s2 {
		t.Errorf("parsered template is not correct")
	}
}

func TestLoadSchematicToTemplate(t *testing.T) {
	testCases := map[string]struct {
		schematic *common.Schematic
		status    *common.Status
		ext       *runtime.RawExtension
		want      *Template
	}{
		"only tmp": {
			schematic: &common.Schematic{CUE: &common.CUE{Template: "t1"}},
			want: &Template{
				TemplateStr:        "t1",
				CapabilityCategory: types.CUECategory,
			},
		},
		"no tmp,but has extension": {
			ext: &runtime.RawExtension{Raw: []byte(`{"template":"t1"}`)},
			want: &Template{
				TemplateStr:        "t1",
				CapabilityCategory: types.CUECategory,
			},
		},
		"no tmp,but has extension without temp": {
			ext: &runtime.RawExtension{Raw: []byte(`{"template":{"t1":"t2"}}`)},
			want: &Template{
				TemplateStr:        "",
				CapabilityCategory: types.CUECategory,
			},
		},
		"tmp with status": {
			schematic: &common.Schematic{CUE: &common.CUE{Template: "t1"}},
			status: &common.Status{
				CustomStatus: "s1",
				HealthPolicy: "h1",
			},
			want: &Template{
				TemplateStr:        "t1",
				CustomStatus:       "s1",
				Health:             "h1",
				CapabilityCategory: types.CUECategory,
			},
		},
		"no tmp only status": {
			status: &common.Status{
				CustomStatus: "s1",
				HealthPolicy: "h1",
			},
			want: &Template{
				CustomStatus: "s1",
				Health:       "h1",
			},
		},
		"terraform schematic": {
			schematic: &common.Schematic{Terraform: &common.Terraform{}},
			want: &Template{
				CapabilityCategory: types.TerraformCategory,
				Terraform:          &common.Terraform{},
			},
		},
	}
	for reason, casei := range testCases {
		gtmp := &Template{}
		err := loadSchematicToTemplate(gtmp, casei.status, casei.schematic, casei.ext)
		assert.NoError(t, err, reason)
		assert.Equal(t, casei.want, gtmp, reason)
	}
}

func TestDryRunTemplateLoader(t *testing.T) {
	compDefStr := `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: myworker
spec:
  status:
    customStatus: testCustomStatus
    healthPolicy: testHealthPolicy
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  schematic:
    cue:
      template: testCUE `

	traitDefStr := `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  name: myingress
spec:
  status:
    customStatus: testCustomStatus
    healthPolicy: testHealthPolicy
  appliesToWorkloads:
    - deployments.apps
  schematic:
    cue:
      template: testCUE `

	compDef, _ := oamutil.UnMarshalStringToComponentDefinition(compDefStr)
	traitDef, _ := oamutil.UnMarshalStringToTraitDefinition(traitDefStr)
	unstrctCompDef, _ := oamutil.Object2Unstructured(compDef)
	unstrctTraitDef, _ := oamutil.Object2Unstructured(traitDef)

	expectedCompTmpl := &Template{
		TemplateStr:        "testCUE",
		Health:             "testHealthPolicy",
		CustomStatus:       "testCustomStatus",
		CapabilityCategory: types.CUECategory,
		Reference: common.WorkloadTypeDescriptor{
			Definition: common.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		},
		ComponentDefinition: compDef,
	}

	expectedTraitTmpl := &Template{
		TemplateStr:        "testCUE",
		Health:             "testHealthPolicy",
		CustomStatus:       "testCustomStatus",
		CapabilityCategory: types.CUECategory,
		TraitDefinition:    traitDef,
	}

	var annotations = make(map[string]string)
	dryRunLoadTemplate := DryRunTemplateLoader([]*unstructured.Unstructured{unstrctCompDef, unstrctTraitDef})
	compTmpl, err := dryRunLoadTemplate(nil, nil, "myworker", types.TypeComponentDefinition, annotations)
	if err != nil {
		t.Error("failed load template of component defintion", err)
	}
	if diff := cmp.Diff(expectedCompTmpl, compTmpl); diff != "" {
		t.Fatal("failed load template of component defintion", diff)
	}

	traitTmpl, err := dryRunLoadTemplate(nil, nil, "myingress", types.TypeTrait, annotations)
	if err != nil {
		t.Error("failed load template of component defintion", err)
	}
	if diff := cmp.Diff(expectedTraitTmpl, traitTmpl); diff != "" {
		t.Fatal("failed load template of trait definition ", diff)
	}
}

func TestLoadTemplateFromRevision(t *testing.T) {
	compDef := v1beta1.ComponentDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.ComponentDefinitionKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-comp"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {name: string}"},
			},
			Workload: common.WorkloadTypeDescriptor{
				Definition: common.WorkloadGVK{APIVersion: "v1", Kind: "Pod"},
			},
		},
	}
	traitDef := v1beta1.TraitDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.TraitDefinitionKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-trait"},
		Spec: v1beta1.TraitDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {port: int}"},
			},
		},
	}
	policyDef := v1beta1.PolicyDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.PolicyDefinitionKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-policy"},
		Spec: v1beta1.PolicyDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {replicas: int}"},
			},
		},
	}
	wfStepDef := v1beta1.WorkflowStepDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.WorkflowStepDefinitionKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-step"},
		Spec: v1beta1.WorkflowStepDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {image: string}"},
			},
		},
	}
	wlDef := v1beta1.WorkloadDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1beta1.WorkloadDefinitionKind,
			APIVersion: v1beta1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: "my-workload"},
		Spec: v1beta1.WorkloadDefinitionSpec{
			Reference: common.DefinitionReference{
				Name: "deployments.apps",
			},
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "output: {apiVersion: 'apps/v1', kind: 'Deployment'}"},
			},
		},
	}

	appRev := &v1beta1.ApplicationRevision{
		ObjectMeta: metav1.ObjectMeta{Name: "my-app-rev"},
		Spec: v1beta1.ApplicationRevisionSpec{
			ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
				ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{
					"my-comp": &compDef,
				},
				TraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"my-trait": &traitDef,
				},
				PolicyDefinitions: map[string]v1beta1.PolicyDefinition{
					"my-policy": policyDef,
				},
				WorkflowStepDefinitions: map[string]*v1beta1.WorkflowStepDefinition{
					"my-step": &wfStepDef,
				},
				WorkloadDefinitions: map[string]v1beta1.WorkloadDefinition{
					"my-workload": wlDef,
				},
			},
		},
	}

	mapper := fakeRESTMapper{}

	testCases := map[string]struct {
		capName   string
		capType   types.CapType
		apprev    *v1beta1.ApplicationRevision
		checkFunc func(t *testing.T, tmpl *Template, err error)
	}{
		"load component definition": {
			capName: "my-comp",
			capType: types.TypeComponentDefinition,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "parameter: {name: string}", tmpl.TemplateStr)
				assert.Equal(t, v1beta1.ComponentDefinitionKind, tmpl.ComponentDefinition.Kind)
			},
		},
		"load trait definition": {
			capName: "my-trait",
			capType: types.TypeTrait,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "parameter: {port: int}", tmpl.TemplateStr)
				assert.Equal(t, v1beta1.TraitDefinitionKind, tmpl.TraitDefinition.Kind)
			},
		},
		"load policy definition": {
			capName: "my-policy",
			capType: types.TypePolicy,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "parameter: {replicas: int}", tmpl.TemplateStr)
				assert.Equal(t, v1beta1.PolicyDefinitionKind, tmpl.PolicyDefinition.Kind)
			},
		},
		"load workflow step definition": {
			capName: "my-step",
			capType: types.TypeWorkflowStep,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "parameter: {image: string}", tmpl.TemplateStr)
				assert.Equal(t, v1beta1.WorkflowStepDefinitionKind, tmpl.WorkflowStepDefinition.Kind)
			},
		},
		"fallback to workload definition": {
			capName: "my-workload",
			capType: types.TypeComponentDefinition,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "output: {apiVersion: 'apps/v1', kind: 'Deployment'}", tmpl.TemplateStr)
				assert.NotNil(t, tmpl.WorkloadDefinition)
				assert.Equal(t, v1beta1.WorkloadDefinitionKind, tmpl.WorkloadDefinition.Kind)
				assert.Equal(t, "apps/v1", tmpl.Reference.Definition.APIVersion)
				assert.Equal(t, "Deployment", tmpl.Reference.Definition.Kind)
			},
		},
		"definition not found": {
			capName: "not-exist",
			capType: types.TypeComponentDefinition,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.Error(t, err)
				assert.Nil(t, tmpl)
				assert.True(t, IsNotFoundInAppRevision(err))
				assert.Contains(t, err.Error(), "component definition [not-exist] not found in app revision my-app-rev")
			},
		},
		"nil app revision": {
			capName: "any",
			capType: types.TypeComponentDefinition,
			apprev:  nil,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.Error(t, err)
				assert.Nil(t, tmpl)
				assert.Contains(t, err.Error(), "fail to find template for any as app revision is empty")
			},
		},
		"unsupported type": {
			capName: "any",
			capType: "unsupported",
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.Error(t, err)
				assert.Nil(t, tmpl)
				assert.Contains(t, err.Error(), "kind(unsupported) of any not supported")
			},
		},
		"verify revision name": {
			capName: "my-comp@my-ns",
			capType: types.TypeComponentDefinition,
			apprev:  appRev,
			checkFunc: func(t *testing.T, tmpl *Template, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, tmpl)
				assert.Equal(t, "parameter: {name: string}", tmpl.TemplateStr)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tmpl, err := LoadTemplateFromRevision(tc.capName, tc.capType, tc.apprev, mapper)
			tc.checkFunc(t, tmpl, err)
		})
	}
}

func TestConvertTemplateJSON2Object(t *testing.T) {
	testCases := map[string]struct {
		capName   string
		in        *runtime.RawExtension
		schematic *common.Schematic
		wantCap   types.Capability
		wantErr   bool
	}{
		"with schematic CUE": {
			capName: "test-cap",
			schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {name: string}"},
			},
			wantCap: types.Capability{
				Name:        "test-cap",
				CueTemplate: "parameter: {name: string}",
			},
			wantErr: false,
		},
		"with RawExtension": {
			capName: "test-cap-2",
			in: &runtime.RawExtension{
				Raw: []byte(`{"template": "parameter: {age: int}"}`),
			},
			wantCap: types.Capability{
				Name:        "test-cap-2",
				CueTemplate: "parameter: {age: int}",
			},
			wantErr: false,
		},
		"with both schematic and RawExtension": {
			capName: "test-cap-3",
			in: &runtime.RawExtension{
				Raw: []byte(`{"description": "test"}`),
			},
			schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {name: string}"},
			},
			wantCap: types.Capability{
				Name:        "test-cap-3",
				Description: "test",
				CueTemplate: "parameter: {name: string}",
			},
			wantErr: false,
		},
		"with invalid JSON in RawExtension": {
			capName: "test-cap-4",
			in: &runtime.RawExtension{
				Raw: []byte(`{"template": "parameter: {age: int}"`),
			},
			wantErr: true,
		},
		"with no template": {
			capName: "test-cap-5",
			in:      &runtime.RawExtension{Raw: []byte(`{"description": "test"}`)},
			wantCap: types.Capability{
				Name:        "test-cap-5",
				Description: "test",
			},
			wantErr: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cap, err := ConvertTemplateJSON2Object(tc.capName, tc.in, tc.schematic)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if diff := cmp.Diff(tc.wantCap, cap); diff != "" {
					t.Errorf("ConvertTemplateJSON2Object() (-want, +got):\n%s", diff)
				}
			}
		})
	}
}

func TestTemplateAsStatusRequest(t *testing.T) {
	tmpl := &Template{
		Health:       "isHealth: true",
		CustomStatus: "message: 'Ready'",
		Details:      "details: 'some details'",
	}
	params := map[string]interface{}{
		"param1": "value1",
	}
	statusReq := tmpl.AsStatusRequest(params)

	assert.Equal(t, "isHealth: true", statusReq.Health)
	assert.Equal(t, "message: 'Ready'", statusReq.Custom)
	assert.Equal(t, "details: 'some details'", statusReq.Details)
	assert.Equal(t, params, statusReq.Parameter)
}

func TestIsNotFoundInAppRevision(t *testing.T) {
	testCases := map[string]struct {
		err      error
		expected bool
	}{
		"component definition not found": {
			err:      fmt.Errorf("component definition [my-comp] not found in app revision [my-app-rev]"),
			expected: true,
		},
		"trait definition not found": {
			err:      fmt.Errorf("trait definition [my-trait] not found in app revision [my-app-rev]"),
			expected: true,
		},
		"policy definition not found": {
			err:      fmt.Errorf("policy definition [my-policy] not found in app revision [my-app-rev]"),
			expected: true,
		},
		"workflow step definition not found": {
			err:      fmt.Errorf("workflow step definition [my-step] not found in app revision [my-app-rev]"),
			expected: true,
		},
		"different error": {
			err:      errors.New("a completely different error"),
			expected: false,
		},
		"nil error": {
			err:      nil,
			expected: false,
		},
		"error with similar text but not exactly": {
			err:      fmt.Errorf("this resource is not found in revision of app"),
			expected: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			result := IsNotFoundInAppRevision(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
