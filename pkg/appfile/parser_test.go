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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	common2 "github.com/oam-dev/kubevela/pkg/utils/common"
)

func TestParsePolicies(t *testing.T) {
	overrideCompDef := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
		Spec:       v1beta1.ComponentDefinitionSpec{Workload: common.WorkloadTypeDescriptor{Type: "Deployment"}},
	}
	customPolicyDef := &v1beta1.PolicyDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-policy", Namespace: "vela-system"},
		Spec: v1beta1.PolicyDefinitionSpec{
			Schematic: &common.Schematic{
				CUE: &common.CUE{Template: "parameter: {name: string}"},
			},
		},
	}
	schemes := runtime.NewScheme()
	v1beta1.AddToScheme(schemes)

	testcases := []struct {
		name           string
		appfile        *Appfile
		client         client.Client
		wantErrContain string
		assertFunc     func(*testing.T, *Appfile)
	}{
		{
			name: "policy with nil properties",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Policies: []v1beta1.AppPolicy{
							{
								Name:       "gc-policy",
								Type:       v1alpha1.GarbageCollectPolicyType,
								Properties: nil,
							},
						},
					},
				},
			},
			client:         fake.NewClientBuilder().WithScheme(schemes).Build(),
			wantErrContain: "must not have empty properties",
		},
		{
			name: "debug policy",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Policies: []v1beta1.AppPolicy{
							{
								Name: "debug-policy",
								Type: v1alpha1.DebugPolicyType,
							},
						},
					},
				},
			},
			client: fake.NewClientBuilder().WithScheme(schemes).Build(),
			assertFunc: func(t *testing.T, af *Appfile) {
				assert.True(t, af.Debug)
			},
		},
		{
			name: "override policy fails to get definition",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []common.ApplicationComponent{
							{
								Name: "comp1",
								Type: "webservice",
							},
						},
						Policies: []v1beta1.AppPolicy{
							{
								Name: "override-policy",
								Type: v1alpha1.OverridePolicyType,
								Properties: util.Object2RawExtension(v1alpha1.OverridePolicySpec{
									Components: []v1alpha1.EnvComponentPatch{
										{
											Name: "comp1",
											Type: "webservice",
										},
									},
								}),
							},
						},
					},
				},
				RelatedComponentDefinitions: make(map[string]*v1beta1.ComponentDefinition),
				RelatedTraitDefinitions:     make(map[string]*v1beta1.TraitDefinition),
			},
			client: &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					return fmt.Errorf("get definition error")
				},
			},
			wantErrContain: "get definition error",
		},
		{
			name: "override policy success",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Components: []common.ApplicationComponent{
							{
								Name: "comp1",
								Type: "webservice",
							},
						},
						Policies: []v1beta1.AppPolicy{
							{
								Name: "override-policy",
								Type: v1alpha1.OverridePolicyType,
								Properties: util.Object2RawExtension(v1alpha1.OverridePolicySpec{
									Components: []v1alpha1.EnvComponentPatch{
										{
											Name: "comp1",
											Type: "webservice",
										},
									},
								}),
							},
						},
					},
				},
				RelatedComponentDefinitions: make(map[string]*v1beta1.ComponentDefinition),
				RelatedTraitDefinitions:     make(map[string]*v1beta1.TraitDefinition),
			},
			client: fake.NewClientBuilder().WithScheme(schemes).WithObjects(overrideCompDef).Build(),
			assertFunc: func(t *testing.T, af *Appfile) {
				assert.Contains(t, af.RelatedComponentDefinitions, "webservice")
			},
		},
		{
			name: "custom policy definition not found",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Policies: []v1beta1.AppPolicy{
							{
								Name:       "my-policy",
								Type:       "custom-policy",
								Properties: util.Object2RawExtension(map[string]string{"name": "test"}),
							},
						},
					},
				},
			},
			client: &test.MockClient{
				MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
					if _, ok := obj.(*v1beta1.PolicyDefinition); ok {
						return errors2.NewNotFound(v1beta1.Resource("policydefinition"), "custom-policy")
					}
					return nil
				},
			},
			wantErrContain: "fetch component/policy type of my-policy",
		},
		{
			name: "custom policy success",
			appfile: &Appfile{
				app: &v1beta1.Application{
					Spec: v1beta1.ApplicationSpec{
						Policies: []v1beta1.AppPolicy{
							{
								Name:       "my-policy",
								Type:       "custom-policy",
								Properties: util.Object2RawExtension(map[string]string{"name": "test"}),
							},
						},
					},
				},
			},
			client: fake.NewClientBuilder().WithScheme(schemes).WithObjects(customPolicyDef).Build(),
			assertFunc: func(t *testing.T, af *Appfile) {
				assert.Equal(t, 1, len(af.ParsedPolicies))
				assert.Equal(t, "my-policy", af.ParsedPolicies[0].Name)
				assert.Equal(t, "custom-policy", af.ParsedPolicies[0].Type)
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewApplicationParser(tc.client)
			// This function is tested separated, mock it for parsePolicies
			if tc.appfile.app != nil {
				tc.appfile.Policies = tc.appfile.app.Spec.Policies
			}
			err := p.parsePolicies(context.Background(), tc.appfile)

			if tc.wantErrContain != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContain)
			} else {
				assert.NoError(t, err)
				if tc.assertFunc != nil {
					tc.assertFunc(t, tc.appfile)
				}
			}
		})
	}
}

var expectedExceptApp = &Appfile{
	Name: "application-sample",
	ParsedComponents: []*Component{
		{
			Name: "myweb",
			Type: "worker",
			Params: map[string]interface{}{
				"image": "busybox",
				"cmd":   []interface{}{"sleep", "1000"},
			},
			FullTemplate: &Template{
				TemplateStr: `
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
      }`,
			},
		},
	},
	WorkflowSteps: []workflowv1alpha1.WorkflowStep{
		{
			WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
				Name: "suspend",
				Type: "suspend",
			},
		},
	},
}

const componentDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: ComponentDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Long-running scalable backend worker without network endpoint"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
  extension:
    template: |
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
      }`

const policyDefinition = `
# Code generated by KubeVela templates. DO NOT EDIT. Please edit the original cue file.
# Definition source cue file: vela-templates/definitions/internal/topology.cue
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  annotations:
    definition.oam.dev/description: Determining the destination where components should be deployed to.
  name: topology
  namespace: {{ include "systemDefinitionNamespace" . }}
spec:
  schematic:
    cue:
      template: |
        parameter: {
        	// +usage=Specify the names of the clusters to select.
        	cluster?: [...string]
        	// +usage=Specify the label selector for clusters
        	clusterLabelSelector?: [string]: string
        	// +usage=Deprecated: Use clusterLabelSelector instead.
        	clusterSelector?: [string]: string
        	// +usage=Specify the target namespace to deploy in the selected clusters, default inherit the original namespace.
        	namespace?: string
        }
`

const appfileYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
  namespace: default
spec:
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
  workflow:
    steps:
    - name: "suspend"
      type: "suspend" 
`

const appfileYaml2 = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
  namespace: default
spec:
  components:
    - name: myweb
      type: worker-notexist
      properties:
        image: "busybox"
`

const appfileYamlEmptyPolicy = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
  namespace: default
spec:
  components: []
  policies:
    - type: garbage-collect
      name: somename
      properties:
`

func TestApplicationParser(t *testing.T) {
	t.Run("Test parse an application", func(t *testing.T) {
		o := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(appfileYaml), &o)
		assert.NoError(t, err)

		// Create a mock client
		tclient := test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if strings.Contains(key.Name, "notexist") {
					return &errors2.StatusError{ErrStatus: metav1.Status{Reason: "NotFound", Message: "not found"}}
				}
				switch o := obj.(type) {
				case *v1beta1.ComponentDefinition:
					wd, err := util.UnMarshalStringToComponentDefinition(componentDefinition)
					if err != nil {
						return err
					}
					*o = *wd
				case *v1beta1.PolicyDefinition:
					ppd, err := util.UnMarshalStringToPolicyDefinition(policyDefinition)
					if err != nil {
						return err
					}
					*o = *ppd
				}
				return nil
			},
		}

		appfile, err := NewApplicationParser(&tclient).GenerateAppFile(context.TODO(), &o)
		assert.NoError(t, err)
		assert.True(t, equal(expectedExceptApp, appfile))

		notfound := v1beta1.Application{}
		err = yaml.Unmarshal([]byte(appfileYaml2), &notfound)
		assert.NoError(t, err)
		_, err = NewApplicationParser(&tclient).GenerateAppFile(context.TODO(), &notfound)
		assert.Error(t, err)

		t.Log("app with empty policy")
		emptyPolicy := v1beta1.Application{}
		err = yaml.Unmarshal([]byte(appfileYamlEmptyPolicy), &emptyPolicy)
		assert.NoError(t, err)
		_, err = NewApplicationParser(&tclient).GenerateAppFile(context.TODO(), &emptyPolicy)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "have empty properties")
	})
}

func equal(af, dest *Appfile) bool {
	if af.Name != dest.Name || len(af.ParsedComponents) != len(dest.ParsedComponents) {
		return false
	}
	for i, wd := range af.ParsedComponents {
		destWd := dest.ParsedComponents[i]
		if wd.Name != destWd.Name || len(wd.Traits) != len(destWd.Traits) {
			return false
		}
		if !reflect.DeepEqual(wd.Params, destWd.Params) {
			fmt.Printf("%#v | %#v\n", wd.Params, destWd.Params)
			return false
		}
		for j, td := range wd.Traits {
			destTd := destWd.Traits[j]
			if td.Name != destTd.Name {
				fmt.Printf("td:%s dest%s", td.Name, destTd.Name)
				return false
			}
			if !reflect.DeepEqual(td.Params, destTd.Params) {
				fmt.Printf("%#v | %#v\n", td.Params, destTd.Params)
				return false
			}
		}
	}
	return true
}

func TestApplicationParserWithLegacyRevision(t *testing.T) {
	var app v1beta1.Application
	var apprev v1beta1.ApplicationRevision
	var wsd v1beta1.WorkflowStepDefinition
	var expectedExceptAppfile *Appfile
	var mockClient test.MockClient

	// prepare WorkflowStepDefinition
	assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/wsd.yaml", &wsd))

	// prepare verify data
	expectedExceptAppfile = &Appfile{
		Name: "backport-1-2-test-demo",
		ParsedComponents: []*Component{
			{
				Name: "backport-1-2-test-demo",
				Type: "webservice",
				Params: map[string]interface{}{
					"image": "nginx",
				},
				FullTemplate: &Template{
					TemplateStr: `
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
      }`,
				},
				Traits: []*Trait{
					{
						Name: "scaler",
						Params: map[string]interface{}{
							"replicas": float64(1),
						},
						Template: `
parameter: {
	// +usage=Specify the number of workload
	replicas: *1 | int
}
// +patchStrategy=retainKeys
patch: spec: replicas: parameter.replicas

`,
					},
				},
			},
		},
		WorkflowSteps: []workflowv1alpha1.WorkflowStep{
			{
				WorkflowStepBase: workflowv1alpha1.WorkflowStepBase{
					Name: "apply",
					Type: "apply-application",
				},
			},
		},
	}

	// Create mock client
	mockClient = test.MockClient{
		MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
			if strings.Contains(key.Name, "unknown") {
				return &errors2.StatusError{ErrStatus: metav1.Status{Reason: "NotFound", Message: "not found"}}
			}
			switch o := obj.(type) {
			case *v1beta1.ComponentDefinition:
				wd, err := util.UnMarshalStringToComponentDefinition(componentDefinition)
				if err != nil {
					return err
				}
				*o = *wd
			case *v1beta1.WorkflowStepDefinition:
				*o = wsd
			case *v1beta1.ApplicationRevision:
				*o = apprev
			default:
				// skip
			}
			return nil
		},
	}

	t.Run("with apply-application workflowStep", func(t *testing.T) {
		// prepare application
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/app.yaml", &app))
		// prepare application revision
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/apprev1.yaml", &apprev))

		t.Run("Test we can parse an application revision to an appFile 1", func(t *testing.T) {

			appfile, err := NewApplicationParser(&mockClient).GenerateAppFile(context.TODO(), &app)
			assert.NoError(t, err)
			assert.True(t, equal(expectedExceptAppfile, appfile))
			assert.True(t, len(appfile.WorkflowSteps) > 0 &&
				len(appfile.RelatedWorkflowStepDefinitions) == len(appfile.AppRevision.Spec.WorkflowStepDefinitions))

			assert.True(t, len(appfile.WorkflowSteps) > 0 && func() bool {
				this := appfile.RelatedWorkflowStepDefinitions
				that := appfile.AppRevision.Spec.WorkflowStepDefinitions
				for i, w := range this {
					thatW := that[i]
					if !reflect.DeepEqual(w, thatW) {
						return false
					}
				}
				return true
			}())
		})
	})

	t.Run("with apply-application and apply-component build-in workflowStep", func(t *testing.T) {
		// prepare application
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/app.yaml", &app))
		// prepare application revision
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/apprev2.yaml", &apprev))

		t.Run("Test we can parse an application revision to an appFile 2", func(t *testing.T) {

			appfile, err := NewApplicationParser(&mockClient).GenerateAppFile(context.TODO(), &app)
			assert.NoError(t, err)
			assert.True(t, equal(expectedExceptAppfile, appfile))
			assert.True(t, len(appfile.WorkflowSteps) > 0 &&
				len(appfile.RelatedWorkflowStepDefinitions) == len(appfile.AppRevision.Spec.WorkflowStepDefinitions))

			assert.True(t, len(appfile.WorkflowSteps) > 0 && func() bool {
				this := appfile.RelatedWorkflowStepDefinitions
				that := appfile.AppRevision.Spec.WorkflowStepDefinitions
				for i, w := range this {
					thatW := that[i]
					if !reflect.DeepEqual(w, thatW) {
						fmt.Printf("appfile wsd:%s apprev wsd%s", (*w).Name, thatW.Name)
						return false
					}
				}
				return true
			}())
		})
	})

	t.Run("with unknown workflowStep", func(t *testing.T) {
		// prepare application
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/app.yaml", &app))
		// prepare application revision
		assert.NoError(t, common2.ReadYamlToObject("testdata/backport-1-2/apprev3.yaml", &apprev))

		t.Run("Test we can parse an application revision to an appFile 3", func(t *testing.T) {

			_, err := NewApplicationParser(&mockClient).GenerateAppFile(context.TODO(), &app)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get workflow step definition apply-application-unknown: not found")
			assert.Contains(t, err.Error(), "failed to parseWorkflowStepsForLegacyRevision")
		})
	})
}

func TestParser_parseTraits(t *testing.T) {
	type args struct {
		workload *Component
		comp     common.ApplicationComponent
	}
	tests := []struct {
		name                 string
		args                 args
		wantErr              assert.ErrorAssertionFunc
		mockTemplateLoaderFn TemplateLoaderFn
		validateFunc         func(w *Component) bool
	}{
		{
			name: "test empty traits",
			args: args{
				comp: common.ApplicationComponent{},
			},
			wantErr: assert.NoError,
		},
		{
			name: "test parse trait properties error",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type: "expose",
							Properties: &runtime.RawExtension{
								Raw: []byte("invalid properties"),
							},
						},
					},
				},
			},
			wantErr: assert.Error,
		},
		{
			name: "test parse trait error",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type: "expose",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"unsupported": "{\"key\":\"value\"}"}`),
							},
						},
					},
				},
			},
			mockTemplateLoaderFn: func(context.Context, client.Client, string, types.CapType, map[string]string) (*Template, error) {
				return nil, fmt.Errorf("unsupported key not found")
			},
			wantErr: assert.Error,
		},
		{
			name: "test parse trait success",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type: "expose",
							Properties: &runtime.RawExtension{
								Raw: []byte(`{"annotation": "{\"key\":\"value\"}"}`),
							},
						},
					},
				},
				workload: &Component{},
			},
			wantErr: assert.NoError,
			mockTemplateLoaderFn: func(ctx context.Context, reader client.Client, s string, capType types.CapType, annotations map[string]string) (*Template, error) {
				return &Template{
					TemplateStr:        "template",
					CapabilityCategory: "network",
					Health:             "true",
					CustomStatus:       "healthy",
				}, nil
			},
			validateFunc: func(w *Component) bool {
				return w != nil && len(w.Traits) != 0 && w.Traits[0].Name == "expose" && w.Traits[0].Template == "template"
			},
		},
	}

	p := NewApplicationParser(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p.tmplLoader = tt.mockTemplateLoaderFn
			annotations := make(map[string]string)
			err := p.parseTraits(context.Background(), tt.args.workload, tt.args.comp, annotations)
			tt.wantErr(t, err, fmt.Sprintf("parseTraits(%v, %v)", tt.args.workload, tt.args.comp))
			if tt.validateFunc != nil {
				assert.True(t, tt.validateFunc(tt.args.workload))
			}
		})
	}
}

func TestParser_parseTraitsFromRevision(t *testing.T) {
	type args struct {
		comp     common.ApplicationComponent
		appRev   *v1beta1.ApplicationRevision
		workload *Component
	}
	tests := []struct {
		name         string
		args         args
		validateFunc func(w *Component) bool
		wantErr      assert.ErrorAssertionFunc
	}{
		{
			name: "test empty traits",
			args: args{
				comp: common.ApplicationComponent{},
			},
			wantErr: assert.NoError,
		},
		{
			name: "test parse traits properties error",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type:       "expose",
							Properties: &runtime.RawExtension{Raw: []byte("invalid")},
						},
					},
				},
				workload: &Component{},
			},
			wantErr: assert.Error,
		},
		{
			name: "test parse traits from revision failed",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type:       "expose",
							Properties: &runtime.RawExtension{Raw: []byte(`{"appRevisionName": "appRevName"}`)},
						},
					},
				},
				appRev: &v1beta1.ApplicationRevision{
					Spec: v1beta1.ApplicationRevisionSpec{
						ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
							TraitDefinitions: map[string]*v1beta1.TraitDefinition{},
						},
					},
				},
				workload: &Component{},
			},
			wantErr: assert.Error,
		},
		{
			name: "test parse traits from revision success",
			args: args{
				comp: common.ApplicationComponent{
					Traits: []common.ApplicationTrait{
						{
							Type:       "expose",
							Properties: &runtime.RawExtension{Raw: []byte(`{"appRevisionName": "appRevName"}`)},
						},
					},
				},
				appRev: &v1beta1.ApplicationRevision{
					Spec: v1beta1.ApplicationRevisionSpec{
						ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
							TraitDefinitions: map[string]*v1beta1.TraitDefinition{
								"expose": {
									Spec: v1beta1.TraitDefinitionSpec{
										RevisionEnabled:    true,
										AppliesToWorkloads: []string{"*"},
									},
								},
							},
						},
					},
				},
				workload: &Component{},
			},
			wantErr: assert.NoError,
			validateFunc: func(w *Component) bool {
				return w != nil && len(w.Traits) == 1 && w.Traits[0].Name == "expose"
			},
		},
	}
	p := NewApplicationParser(fake.NewClientBuilder().Build())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.wantErr(t, p.parseTraitsFromRevision(tt.args.comp, tt.args.appRev, tt.args.workload), fmt.Sprintf("parseTraitsFromRevision(%v, %v, %v)", tt.args.comp, tt.args.appRev, tt.args.workload))
			if tt.validateFunc != nil {
				assert.True(t, tt.validateFunc(tt.args.workload))
			}
		})
	}
}

func TestParseComponentFromRevisionAndClient(t *testing.T) {
	compDef := &v1beta1.ComponentDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "webservice", Namespace: "vela-system"},
		Spec: v1beta1.ComponentDefinitionSpec{
			Workload:  common.WorkloadTypeDescriptor{Type: "Deployment"},
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "parameter: {image: string}"}},
		},
	}
	traitDef := &v1beta1.TraitDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "scaler", Namespace: "vela-system"},
		Spec: v1beta1.TraitDefinitionSpec{
			Schematic: &common.Schematic{CUE: &common.CUE{Template: "parameter: {replicas: int}"}},
		},
	}

	appComp := common.ApplicationComponent{
		Name:       "my-comp",
		Type:       "webservice",
		Properties: util.Object2RawExtension(map[string]string{"image": "nginx"}),
		Traits: []common.ApplicationTrait{
			{
				Type:       "scaler",
				Properties: util.Object2RawExtension(map[string]int{"replicas": 2}),
			},
		},
	}

	schemes := runtime.NewScheme()
	v1beta1.AddToScheme(schemes)

	tests := []struct {
		name       string
		appRev     *v1beta1.ApplicationRevision
		client     client.Client
		wantErr    bool
		assertFunc func(*testing.T, *Component)
	}{
		{
			name: "component and trait found in revision",
			appRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{"webservice": compDef},
						TraitDefinitions:     map[string]*v1beta1.TraitDefinition{"scaler": traitDef},
					},
				},
			},
			client:  fake.NewClientBuilder().WithScheme(schemes).Build(),
			wantErr: false,
			assertFunc: func(t *testing.T, c *Component) {
				assert.NotNil(t, c)
				assert.Equal(t, "my-comp", c.Name)
				assert.Equal(t, "webservice", c.Type)
				assert.Equal(t, 1, len(c.Traits))
				assert.Equal(t, "scaler", c.Traits[0].Name)
			},
		},
		{
			name: "component not in revision, but in cluster",
			appRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						TraitDefinitions: map[string]*v1beta1.TraitDefinition{"scaler": traitDef},
					},
				},
			},
			client:  fake.NewClientBuilder().WithScheme(schemes).WithObjects(compDef).Build(),
			wantErr: false,
			assertFunc: func(t *testing.T, c *Component) {
				assert.NotNil(t, c)
				assert.Equal(t, "webservice", c.Type)
				assert.Equal(t, 1, len(c.Traits))
				assert.Equal(t, "scaler", c.Traits[0].Name)
			},
		},
		{
			name: "trait not in revision, but in cluster",
			appRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{"webservice": compDef},
					},
				},
			},
			client:  fake.NewClientBuilder().WithScheme(schemes).WithObjects(traitDef).Build(),
			wantErr: false,
			assertFunc: func(t *testing.T, c *Component) {
				assert.NotNil(t, c)
				assert.Equal(t, "webservice", c.Type)
				assert.Equal(t, 1, len(c.Traits))
				assert.Equal(t, "scaler", c.Traits[0].Name)
			},
		},
		{
			name:    "component and trait not in revision, but in cluster",
			appRev:  &v1beta1.ApplicationRevision{},
			client:  fake.NewClientBuilder().WithScheme(schemes).WithObjects(compDef, traitDef).Build(),
			wantErr: false,
			assertFunc: func(t *testing.T, c *Component) {
				assert.NotNil(t, c)
				assert.Equal(t, "webservice", c.Type)
				assert.Equal(t, 1, len(c.Traits))
				assert.Equal(t, "scaler", c.Traits[0].Name)
			},
		},
		{
			name:    "component not found anywhere",
			appRev:  &v1beta1.ApplicationRevision{},
			client:  fake.NewClientBuilder().WithScheme(schemes).Build(),
			wantErr: true,
		},
		{
			name: "trait not found anywhere",
			appRev: &v1beta1.ApplicationRevision{
				Spec: v1beta1.ApplicationRevisionSpec{
					ApplicationRevisionCompressibleFields: v1beta1.ApplicationRevisionCompressibleFields{
						ComponentDefinitions: map[string]*v1beta1.ComponentDefinition{"webservice": compDef},
					},
				},
			},
			client:  fake.NewClientBuilder().WithScheme(schemes).Build(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewApplicationParser(tt.client)
			comp, err := p.ParseComponentFromRevisionAndClient(context.Background(), appComp, tt.appRev)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.assertFunc != nil {
					tt.assertFunc(t, comp)
				}
			}
		})
	}
}
