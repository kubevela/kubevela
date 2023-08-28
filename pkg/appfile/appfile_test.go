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
	"encoding/json"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/model"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Workflow", func() {
	It("generate workflow task runners", func() {
		workflowStepDef := v1beta1.WorkflowStepDefinition{
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &common.Schematic{
					CUE: &common.CUE{
						Template: `
wait: op.#ConditionalWait & {
  continue: true
}
`,
					},
				},
			},
		}
		workflowStepDef.Name = "test-wait"
		workflowStepDef.Namespace = "default"
		err := k8sClient.Create(context.Background(), &workflowStepDef)
		Expect(err).To(BeNil())

		notCueStepDef := v1beta1.WorkflowStepDefinition{
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &common.Schematic{},
			},
		}

		notCueStepDef.Name = "not-cue"
		notCueStepDef.Namespace = "default"
		err = k8sClient.Create(context.Background(), &notCueStepDef)
		Expect(err).To(BeNil())
	})
})

var _ = Describe("Test Terraform schematic appfile", func() {
	It("workload capability is Terraform", func() {
		var (
			ns            = "default"
			compName      = "sample-db"
			appName       = "webapp"
			revision      = "v1"
			configuration = `
module "rds" {
  source = "terraform-alicloud-modules/rds/alicloud"
  engine = "MySQL"
  engine_version = "8.0"
  instance_type = "rds.mysql.c1.large"
  instance_storage = "20"
  instance_name = var.instance_name
  account_name = var.account_name
  password = var.password
}

output "DB_NAME" {
  value = module.rds.this_db_instance_name
}
output "DB_USER" {
  value = module.rds.this_db_database_account
}
output "DB_PORT" {
  value = module.rds.this_db_instance_port
}
output "DB_HOST" {
  value = module.rds.this_db_instance_connection_string
}
output "DB_PASSWORD" {
  value = module.rds.this_db_instance_port
}

variable "instance_name" {
  description = "RDS instance name"
  type = string
  default = "poc"
}

variable "account_name" {
  description = "RDS instance user account name"
  type = "string"
  default = "oam"
}

variable "password" {
  description = "RDS instance account password"
  type = "string"
  default = "Xyfff83jfewGGfaked"
}
`
		)

		wl := &Component{
			Name: "sample-db",
			FullTemplate: &Template{
				Terraform: &common.Terraform{
					Configuration: configuration,
					Type:          "hcl",
				},
				ComponentDefinition: &v1beta1.ComponentDefinition{
					Spec: v1beta1.ComponentDefinitionSpec{
						Schematic: &common.Schematic{
							Terraform: &common.Terraform{},
						},
					},
				},
			},
			CapabilityCategory: oamtypes.TerraformCategory,
			Params: map[string]interface{}{
				"account_name": "oamtest",
				"writeConnectionSecretToRef": map[string]interface{}{
					"name": "db",
				},
				process.OutputSecretName: "db-conn",
			},
		}

		af := &Appfile{
			ParsedComponents: []*Component{wl},
			Name:             appName,
			AppRevisionName:  revision,
			Namespace:        ns,
		}

		variable := map[string]interface{}{"account_name": "oamtest"}
		data, _ := json.Marshal(variable)
		raw := &runtime.RawExtension{}
		raw.Raw = data

		workload := terraformapi.Configuration{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app.oam.dev/appRevision": "v1",
					"app.oam.dev/component":   "sample-db",
					"app.oam.dev/name":        "webapp",
					"workload.oam.dev/type":   "",
				},
				Name:      "sample-db",
				Namespace: "default",
			},

			Spec: terraformapi.ConfigurationSpec{
				HCL:      configuration,
				Variable: raw,
			},
			Status: terraformapi.ConfigurationStatus{},
		}
		workload.Spec.WriteConnectionSecretToReference = &terraformtypes.SecretReference{Name: "db", Namespace: "default"}

		expectCompManifest := &oamtypes.ComponentManifest{
			Name: compName,
			ComponentOutput: func() *unstructured.Unstructured {
				r, _ := util.Object2Unstructured(workload)
				return r
			}(),
		}

		comps, err := af.GenerateComponentManifests()
		diff := cmp.Diff(comps[0], expectCompManifest)
		Expect(diff).ShouldNot(BeEmpty())
		Expect(err).Should(BeNil())
	})
})

func TestSetParameterValuesToKubeObj(t *testing.T) {
	tests := map[string]struct {
		reason  string
		obj     unstructured.Unstructured
		values  paramValueSettings
		wantObj unstructured.Unstructured
		wantErr error
	}{
		"InvalidStringType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      int32(100),
					ValueType:  common.StringType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.StringType),
		},
		"InvalidNumberType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"intParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.NumberType),
		},
		"InvalidBoolType": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"boolParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.BooleanType,
					FieldPaths: []string{"spec.test"},
				},
			},
			wantErr: errors.Errorf(errInvalidValueType, common.BooleanType),
		},
		"InvalidFieldPath": {
			reason: "An error should be returned",
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: []string{"spec[.test"}, // a invalid field path
				},
			},
			wantErr: errors.Wrap(errors.New(`cannot parse path "spec[.test": unterminated '[' at position 4`),
				`cannot set parameter "strParam" to field "spec[.test"`),
		},
		"Succeed": {
			reason: "No error should be returned",
			obj:    unstructured.Unstructured{Object: make(map[string]interface{})},
			values: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: []string{"spec.strField"},
				},
				"intParam": paramValueSetting{
					Value:      10,
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.intField"},
				},
				"floatParam": paramValueSetting{
					Value:      float64(10.01),
					ValueType:  common.NumberType,
					FieldPaths: []string{"spec.floatField"},
				},
				"boolParam": paramValueSetting{
					Value:      true,
					ValueType:  common.BooleanType,
					FieldPaths: []string{"spec.boolField"},
				},
			},
			wantObj: unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"strField":   "test",
					"intField":   int64(10),
					"floatField": float64(10.01),
					"boolField":  true,
				},
			}},
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			obj := tc.obj.DeepCopy()
			err := setParameterValuesToKubeObj(obj, tc.values)
			if diff := cmp.Diff(tc.wantObj, *obj); diff != "" {
				t.Errorf("\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
		})
	}

}

var _ = Describe("Test evalWorkloadWithContext", func() {
	It("workload capability is Terraform", func() {
		var (
			ns       = "default"
			compName = "sample-db"
			err      error
		)
		type appArgs struct {
			wl       *Component
			appName  string
			revision string
		}

		args := appArgs{
			wl: &Component{
				Name: compName,
				FullTemplate: &Template{
					Terraform: &common.Terraform{
						Configuration: `
module "rds" {
  source = "terraform-alicloud-modules/rds/alicloud"
  engine = "MySQL"
  engine_version = "8.0"
  instance_type = "rds.mysql.c1.large"
  instance_storage = "20"
  instance_name = var.instance_name
  account_name = var.account_name
  password = var.password
}

output "DB_NAME" {
  value = module.rds.this_db_instance_name
}
output "DB_USER" {
  value = module.rds.this_db_database_account
}
output "DB_PORT" {
  value = module.rds.this_db_instance_port
}
output "DB_HOST" {
  value = module.rds.this_db_instance_connection_string
}
output "DB_PASSWORD" {
  value = module.rds.this_db_instance_port
}

variable "instance_name" {
  description = "RDS instance name"
  type = string
  default = "poc"
}

variable "account_name" {
  description = "RDS instance user account name"
  type = "string"
  default = "oam"
}

variable "password" {
  description = "RDS instance account password"
  type = "string"
  default = "Xyfff83jfewGGfaked"
}
`,
						Type: "hcl",
					},
					ComponentDefinition: &v1beta1.ComponentDefinition{
						Spec: v1beta1.ComponentDefinitionSpec{
							Schematic: &common.Schematic{
								Terraform: &common.Terraform{},
							},
						},
					},
				},
				CapabilityCategory: oamtypes.TerraformCategory,
				engine:             definition.NewWorkloadAbstractEngine(compName, pd),
				Params: map[string]interface{}{
					"variable": map[string]interface{}{
						"account_name": "oamtest",
					},
				},
			},
			appName:  "webapp",
			revision: "v1",
		}

		ctxData := GenerateContextDataFromAppFile(&Appfile{
			Name:            args.appName,
			Namespace:       ns,
			AppRevisionName: args.revision,
		}, args.wl.Name)
		pCtx := NewBasicContext(ctxData, args.wl.Params)
		comp, err := evalWorkloadWithContext(pCtx, args.wl, ns, args.appName)
		Expect(comp.ComponentOutput).ShouldNot(BeNil())
		Expect(comp.Name).Should(Equal(""))
		Expect(err).Should(BeNil())
	})
})

func TestGenerateTerraformConfigurationWorkload(t *testing.T) {
	var (
		name     = "oss"
		ns       = "default"
		variable = map[string]interface{}{"acl": "private"}
	)

	ch := make(chan string)
	badParam := map[string]interface{}{"abc": ch}
	_, badParamMarshalError := json.Marshal(badParam)

	type args struct {
		writeConnectionSecretToRef *terraformtypes.SecretReference
		hcl                        string
		remote                     string
		params                     map[string]interface{}
		providerRef                *terraformtypes.Reference
	}

	type want struct {
		err error
	}

	testcases := map[string]struct {
		args args
		want want
	}{
		"invalid ComponentDefinition": {
			args: args{
				hcl: "abc",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss", "namespace": "default"}},
			},
			want: want{err: errors.New("terraform component definition is not valid")},
		},
		"valid hcl workload": {
			args: args{
				hcl: "abc",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss", "namespace": "default"}},
				writeConnectionSecretToRef: &terraformtypes.SecretReference{Name: "oss", Namespace: "default"},
			},
			want: want{err: nil}},
		"valid hcl workload, and there are some custom params compared to ComponentDefinition": {
			args: args{
				hcl: "def",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss2", "namespace": "default2"},
					"providerRef":                map[string]interface{}{"name": "aws2", "namespace": "default2"}},
				writeConnectionSecretToRef: &terraformtypes.SecretReference{Name: "oss", Namespace: "default"},
				providerRef:                &terraformtypes.Reference{Name: "aws", Namespace: "default"},
			},
			want: want{err: nil},
		},
		"valid hcl workload, but the namespace of WriteConnectionSecretToReference is empty": {
			args: args{
				hcl: "abc",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss", "namespace": "default"}},
				writeConnectionSecretToRef: &terraformtypes.SecretReference{Name: "oss"},
			},
			want: want{err: nil}},

		"remote hcl workload": {
			args: args{
				remote: "https://xxx/a.git",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss", "namespace": "default"}},
				writeConnectionSecretToRef: &terraformtypes.SecretReference{Name: "oss", Namespace: "default"},
			},
			want: want{err: nil}},

		"workload's TF configuration is empty": {
			args: args{
				params: variable,
			},
			want: want{err: errors.New(errTerraformConfigurationIsNotSet)},
		},

		"workload's params is bad": {
			args: args{
				params: badParam,
				hcl:    "abc",
			},
			want: want{err: errors.Wrap(badParamMarshalError, errFailToConvertTerraformComponentProperties)},
		},

		"terraform workload has a provider reference, but parameters are bad": {
			args: args{
				params:      badParam,
				hcl:         "abc",
				providerRef: &terraformtypes.Reference{Name: "azure", Namespace: "default"},
			},
			want: want{err: errors.Wrap(badParamMarshalError, errFailToConvertTerraformComponentProperties)},
		},
		"terraform workload has a provider reference": {
			args: args{
				params:      variable,
				hcl:         "variable \"name\" {\n      description = \"Name to be used on all resources as prefix. Default to 'TF-Module-EIP'.\"\n      default = \"TF-Module-EIP\"\n      type = string\n    }",
				providerRef: &terraformtypes.Reference{Name: "aws", Namespace: "default"},
			},
			want: want{err: nil},
		},
	}

	for tcName, tc := range testcases {
		t.Run(tcName, func(t *testing.T) {

			var (
				template   *Template
				configSpec terraformapi.ConfigurationSpec
			)
			data, _ := json.Marshal(variable)
			raw := &runtime.RawExtension{}
			raw.Raw = data
			if tc.args.hcl != "" {
				template = &Template{
					Terraform: &common.Terraform{
						Configuration: tc.args.hcl,
						Type:          "hcl",
					},
				}
				configSpec = terraformapi.ConfigurationSpec{
					HCL:      tc.args.hcl,
					Variable: raw,
				}
				configSpec.WriteConnectionSecretToReference = tc.args.writeConnectionSecretToRef
			}
			if tc.args.remote != "" {
				template = &Template{
					Terraform: &common.Terraform{
						Configuration: tc.args.remote,
						Type:          "remote",
					},
				}
				configSpec = terraformapi.ConfigurationSpec{
					Remote:   tc.args.remote,
					Variable: raw,
				}
				configSpec.WriteConnectionSecretToReference = tc.args.writeConnectionSecretToRef
			}
			if tc.args.hcl == "" && tc.args.remote == "" {
				template = &Template{
					Terraform: &common.Terraform{},
				}

				configSpec = terraformapi.ConfigurationSpec{
					Variable: raw,
				}
				configSpec.WriteConnectionSecretToReference = tc.args.writeConnectionSecretToRef
			}
			tf := &common.Terraform{}
			if tc.args.providerRef != nil {
				tf.ProviderReference = tc.args.providerRef
				configSpec.ProviderReference = tc.args.providerRef
			}
			if tc.args.writeConnectionSecretToRef != nil {
				tf.WriteConnectionSecretToReference = tc.args.writeConnectionSecretToRef
				configSpec.WriteConnectionSecretToReference = tc.args.writeConnectionSecretToRef
				if tc.args.writeConnectionSecretToRef.Namespace == "" {
					configSpec.WriteConnectionSecretToReference.Namespace = ns
				}
			}

			if tc.args.providerRef != nil || tc.args.writeConnectionSecretToRef != nil {
				template.ComponentDefinition = &v1beta1.ComponentDefinition{
					Spec: v1beta1.ComponentDefinitionSpec{
						Schematic: &common.Schematic{
							Terraform: tf,
						},
					},
				}
			}

			if tc.args.hcl == "def" {
				configSpec.WriteConnectionSecretToReference = &terraformtypes.SecretReference{
					Name:      "oss2",
					Namespace: "default2",
				}
				configSpec.ProviderReference = &terraformtypes.Reference{
					Name:      "aws2",
					Namespace: "default2",
				}
			}

			wl := &Component{
				FullTemplate: template,
				Name:         name,
				Params:       tc.args.params,
			}

			got, err := generateTerraformConfigurationWorkload(wl, ns)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\ngenerateTerraformConfigurationWorkload(...): -want error, +got error:\n%s\n", tcName, diff)
			}

			if err == nil {
				tfConfiguration := terraformapi.Configuration{
					TypeMeta:   metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta2", Kind: "Configuration"},
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
					Spec:       configSpec,
				}
				rawConf := util.Object2RawExtension(tfConfiguration)
				wantWL, _ := util.RawExtension2Unstructured(rawConf)

				if diff := cmp.Diff(wantWL, got); diff != "" {
					t.Errorf("\n%s\ngenerateTerraformConfigurationWorkload(...): -want, +got:\n%s\n", tcName, diff)
				}
			}
		})
	}
}

func TestPrepareArtifactsData(t *testing.T) {
	compManifests := []*oamtypes.ComponentManifest{
		{
			Name:         "readyComp",
			Namespace:    "ns",
			RevisionName: "readyComp-v1",
			ComponentOutput: &unstructured.Unstructured{Object: map[string]interface{}{
				"fake": "workload",
			}},
			ComponentOutputsAndTraits: func() []*unstructured.Unstructured {
				ingressYAML := `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  labels:
    trait.oam.dev/resource: ingress
    trait.oam.dev/type: ingress
  namespace: default
spec:
  rules:
  - host: testsvc.example.com`
				ingress := &unstructured.Unstructured{}
				_ = yaml.Unmarshal([]byte(ingressYAML), ingress)
				svcYAML := `apiVersion: v1
kind: Service
metadata:
  labels:
    trait.oam.dev/resource: service
    trait.oam.dev/type: ingress
  namespace: default
spec:
  clusterIP: 10.96.185.119
  selector:
    app.oam.dev/component: express-server
  type: ClusterIP`
				svc := &unstructured.Unstructured{}
				_ = yaml.Unmarshal([]byte(svcYAML), svc)
				return []*unstructured.Unstructured{ingress, svc}
			}(),
		},
	}

	gotArtifacts := prepareArtifactsData(compManifests)
	gotWorkload, _, err := unstructured.NestedMap(gotArtifacts, "readyComp", "workload")
	assert.NoError(t, err)
	diff := cmp.Diff(gotWorkload, map[string]interface{}{"fake": string("workload")})
	assert.Equal(t, diff, "")

	_, gotIngress, err := unstructured.NestedMap(gotArtifacts, "readyComp", "traits", "ingress", "ingress")
	assert.NoError(t, err)
	if !gotIngress {
		t.Fatalf("cannot get ingress trait")
	}
	_, gotSvc, err := unstructured.NestedMap(gotArtifacts, "readyComp", "traits", "ingress", "service")
	assert.NoError(t, err)
	if !gotSvc {
		t.Fatalf("cannot get service trait")
	}

}

func TestBaseGenerateComponent(t *testing.T) {
	var appName = "test-app"
	var ns = "test-ns"
	var traitName = "mytrait"
	var wlName = "my-wl-1"
	var workflowName = "my-wf"
	var publishVersion = "123"
	ctxData := GenerateContextDataFromAppFile(&Appfile{
		Name:      appName,
		Namespace: ns,
		AppAnnotations: map[string]string{
			oam.AnnotationWorkflowName:   workflowName,
			oam.AnnotationPublishVersion: publishVersion,
		},
	}, wlName)
	pContext := NewBasicContext(ctxData, nil)
	base := `
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		template: {
			spec: containers: [{
				image: "nginx"
			}]
		}
	}
`
	inst := cuecontext.New().CompileString(base)
	bs, _ := model.NewBase(inst.Value())
	err := pContext.SetBase(bs)
	assert.NoError(t, err)
	tr := &Trait{
		Name:   traitName,
		engine: definition.NewTraitAbstractEngine(traitName, nil),
		Template: `outputs:mytrait:{
if context.componentType == "stateless" {
             kind:  			"Deployment"
	}
	if context.componentType  == "stateful" {
             kind:  			"StatefulSet"
	}
	name:                   context.name
	envSourceContainerName: context.name
  workflowName:           context.workflowName
  publishVersion:         context.publishVersion
}`,
	}
	wl := &Component{Type: "stateful", Traits: []*Trait{tr}}
	cm, err := baseGenerateComponent(pContext, wl, appName, ns)
	assert.NoError(t, err)
	assert.Equal(t, cm.ComponentOutputsAndTraits[0].Object["kind"], "StatefulSet")
	assert.Equal(t, cm.ComponentOutputsAndTraits[0].Object["workflowName"], workflowName)
	assert.Equal(t, cm.ComponentOutputsAndTraits[0].Object["publishVersion"], publishVersion)
}

var _ = Describe("Test use context.appLabels& context.appAnnotations in componentDefinition ", func() {
	It("Test generate AppConfig resources from ", func() {
		af := &Appfile{
			Name:      "app",
			Namespace: "ns",
			AppLabels: map[string]string{
				"lk1": "lv1",
				"lk2": "lv2",
			},
			AppAnnotations: map[string]string{
				"ak1": "av1",
				"ak2": "av2",
			},
			ParsedComponents: []*Component{
				{
					Name: "comp1",
					Type: "deployment",
					Params: map[string]interface{}{
						"image": "busybox",
						"cmd":   []interface{}{"sleep", "1000"},
					},
					engine: definition.NewWorkloadAbstractEngine("myweb", pd),
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
									metadata: {
										labels: {
											if context.appLabels != _|_ {
												context.appLabels
											}
										}
										annotations: {
											if context.appAnnotations != _|_ {
												context.appAnnotations
											}
										}
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
						  }`},
				},
			},
		}
		By("Generate ComponentManifests")
		componentManifests, err := af.GenerateComponentManifests()
		Expect(err).To(BeNil())
		By("Verify expected ComponentManifest")
		deployment := &appsv1.Deployment{}
		runtime.DefaultUnstructuredConverter.FromUnstructured(componentManifests[0].ComponentOutput.Object, deployment)
		labels := deployment.Spec.Template.Labels
		annotations := deployment.Spec.Template.Annotations
		Expect(cmp.Diff(len(labels), 2)).Should(BeEmpty())
		Expect(cmp.Diff(len(annotations), 2)).Should(BeEmpty())
		Expect(cmp.Diff(labels["lk1"], "lv1")).Should(BeEmpty())
		Expect(cmp.Diff(annotations["ak1"], "av1")).Should(BeEmpty())
	})

})
