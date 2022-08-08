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
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/mock"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

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
apiVersion: core.oam.dev/v1alpha2
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
	tdm := mock.NewMockDiscoveryMapper()
	tdm.MockKindsFor = mock.NewMockKindsFor("Deployment", "v1")
	temp, err := LoadTemplate(context.TODO(), tdm, &tclient, "worker", types.TypeComponentDefinition)

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

func TestLoadWorkloadTemplate(t *testing.T) {
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

	var workloadDefintion = `
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
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
			case *v1alpha2.WorkloadDefinition:
				cd, err := oamutil.UnMarshalStringToWorkloadDefinition(workloadDefintion)
				if err != nil {
					return err
				}
				*o = *cd
			case *v1alpha2.ComponentDefinition:
				err := mock.NewMockNotFoundErr()
				return err
			}
			return nil
		},
	}
	tdm := mock.NewMockDiscoveryMapper()
	tdm.MockKindsFor = mock.NewMockKindsFor("Deployment", "v1")
	temp, err := LoadTemplate(context.TODO(), tdm, &tclient, "worker", types.TypeComponentDefinition)

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
        	apiVersion: "networking.k8s.io/v1beta1"
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
        						backend: {
        							serviceName: context.name
        							servicePort: v
        						}
        					},
        				]
        			}
        		}]
        	}
        }
      `

	var traitDefintion = `
apiVersion: core.oam.dev/v1alpha2
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

	tdm := mock.NewMockDiscoveryMapper()
	tdm.MockKindsFor = mock.NewMockKindsFor("Deployment", "v1")
	temp, err := LoadTemplate(context.TODO(), tdm, &tclient, "ingress", types.TypeTrait)

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
		"helm schematic": {
			schematic: &common.Schematic{HELM: &common.Helm{}},
			want: &Template{
				CapabilityCategory: types.HelmCategory,
				Helm:               &common.Helm{},
			},
		},
		"kube schematic": {
			schematic: &common.Schematic{KUBE: &common.Kube{}},
			want: &Template{
				CapabilityCategory: types.KubeCategory,
				Kube:               &common.Kube{},
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
apiVersion: core.oam.dev/v1alpha2
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
apiVersion: core.oam.dev/v1alpha2
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
		Helm:                nil,
		Kube:                nil,
		ComponentDefinition: compDef,
	}

	expectedTraitTmpl := &Template{
		TemplateStr:        "testCUE",
		Health:             "testHealthPolicy",
		CustomStatus:       "testCustomStatus",
		CapabilityCategory: types.CUECategory,
		Helm:               nil,
		Kube:               nil,
		TraitDefinition:    traitDef,
	}

	dryRunLoadTemplate := DryRunTemplateLoader([]oam.Object{unstrctCompDef, unstrctTraitDef})
	compTmpl, err := dryRunLoadTemplate(nil, nil, nil, "myworker", types.TypeComponentDefinition)
	if err != nil {
		t.Error("failed load template of component defintion", err)
	}
	if diff := cmp.Diff(expectedCompTmpl, compTmpl); diff != "" {
		t.Fatal("failed load template of component defintion", diff)
	}

	traitTmpl, err := dryRunLoadTemplate(nil, nil, nil, "myingress", types.TypeTrait)
	if err != nil {
		t.Error("failed load template of component defintion", err)
	}
	if diff := cmp.Diff(expectedTraitTmpl, traitTmpl); diff != "" {
		t.Fatal("failed load template of trait definition ", diff)
	}
}
