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
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/kubevela/workflow/pkg/cue/model"

	workflowv1alpha1 "github.com/kubevela/workflow/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

func TestWorkflow(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()
	t.Run("generate workflow task runners", func(t *testing.T) {
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
		assert.NoError(t, err)

		notCueStepDef := v1beta1.WorkflowStepDefinition{
			Spec: v1beta1.WorkflowStepDefinitionSpec{
				Schematic: &common.Schematic{},
			},
		}

		notCueStepDef.Name = "not-cue"
		notCueStepDef.Namespace = "default"
		err = k8sClient.Create(context.Background(), &notCueStepDef)
		assert.NoError(t, err)
	})
}

func TestTerraformSchematicAppfile(t *testing.T) {
	t.Run("workload capability is Terraform", func(t *testing.T) {
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
		assert.NotEmpty(t, diff)
		assert.NoError(t, err)
	})
}

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
				t.Errorf(
					"\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf(
					"\nsetParameterValuesToKubeObj(...)error -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
		})
	}

}

func TestEvalWorkloadWithContext(t *testing.T) {
	t.Run("workload capability is Terraform", func(t *testing.T) {
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
				engine:             definition.NewWorkloadAbstractEngine(compName),
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
		assert.NotNil(t, comp.ComponentOutput)
		assert.Equal(t, "", comp.Name)
		assert.NoError(t, err)
	})
}

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
		engine: definition.NewTraitAbstractEngine(traitName),
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

func TestAppLabelsAndAnnotationsInComponentDefinition(t *testing.T) {
	t.Run("generate AppConfig resources", func(t *testing.T) {
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
					engine: definition.NewWorkloadAbstractEngine("myweb"),
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

		// Generate ComponentManifests
		componentManifests, err := af.GenerateComponentManifests()
		if err != nil {
			t.Fatalf("GenerateComponentManifests() error = %v", err)
		}

		// Verify expected ComponentManifest
		deployment := &appsv1.Deployment{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(
			componentManifests[0].ComponentOutput.Object, deployment); err != nil {
			t.Fatalf("Failed to convert to Deployment: %v", err)
		}

		labels := deployment.Spec.Template.Labels
		annotations := deployment.Spec.Template.Annotations

		if diff := cmp.Diff(len(labels), 2); diff != "" {
			t.Errorf("labels length mismatch (-want +got): %s", diff)
		}
		if diff := cmp.Diff(len(annotations), 2); diff != "" {
			t.Errorf("annotations length mismatch (-want +got): %s", diff)
		}
		if diff := cmp.Diff(labels["lk1"], "lv1"); diff != "" {
			t.Errorf("label mismatch (-want +got): %s", diff)
		}
		if diff := cmp.Diff(annotations["ak1"], "av1"); diff != "" {
			t.Errorf("annotation mismatch (-want +got): %s", diff)
		}
	})
}

func TestIsNotFoundInAppFile(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"ErrorIsNil": {
			err:  nil,
			want: false,
		},
		"ErrorContainsSubstring": {
			err:  errors.New("some error not found in appfile"),
			want: true,
		},
		"ErrorNotContainsSubstring": {
			err:  errors.New("some other error"),
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := IsNotFoundInAppFile(tc.err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPrepareProcessContext(t *testing.T) {
	ctxData := process.ContextData{
		AppName:   "test-app",
		Namespace: "test-ns",
	}

	t.Run("Success case with nil initial context", func(t *testing.T) {
		comp := &Component{
			engine: definition.NewWorkloadAbstractEngine("test"),
			FullTemplate: &Template{
				TemplateStr: `
output: {}
parameter: {name: string}
`,
			},
			Params: map[string]interface{}{"name": "test-name"},
		}
		assert.Nil(t, comp.Ctx)

		ctx, err := PrepareProcessContext(comp, ctxData)

		assert.NoError(t, err)
		assert.NotNil(t, ctx)
		assert.NotNil(t, comp.Ctx)
		assert.Equal(t, ctx, comp.Ctx)
	})

	t.Run("Error case from invalid CUE template", func(t *testing.T) {
		comp := &Component{
			engine: definition.NewWorkloadAbstractEngine("test"),
			FullTemplate: &Template{
				TemplateStr: `
output: {}
parameter: {name: string & int}
`,
			},
			Params: map[string]interface{}{"name": "test-name"},
		}

		_, err := PrepareProcessContext(comp, ctxData)

		assert.Error(t, err)
		assert.ErrorContains(t, err, "conflicting values string and int")
	})

	t.Run("Use existing context if not nil", func(t *testing.T) {
		comp := &Component{
			engine: definition.NewWorkloadAbstractEngine("test"),
			FullTemplate: &Template{
				TemplateStr: `
output: {}
parameter: {name: string}
`,
			},
			Params: map[string]interface{}{"name": "test-name"},
		}

		// First call to populate context
		ctx1, err := PrepareProcessContext(comp, ctxData)
		assert.NoError(t, err)
		assert.NotNil(t, ctx1)

		// Second call
		ctx2, err := PrepareProcessContext(comp, ctxData)
		assert.NoError(t, err)
		assert.NotNil(t, ctx2)

		// Check if the context object is the same
		assert.Same(t, ctx1, ctx2)
	})
}

func TestLoadDynamicComponent(t *testing.T) {
	ctx := context.Background()
	cli := fake.NewClientBuilder().Build()

	t.Run("Non-ref-objects component", func(t *testing.T) {
		af := &Appfile{}
		comp := &common.ApplicationComponent{
			Type: "webservice",
		}
		resultComp, err := af.LoadDynamicComponent(ctx, cli, comp)
		assert.NoError(t, err)
		assert.Equal(t, comp, resultComp)
	})

	t.Run("Invalid properties", func(t *testing.T) {
		af := &Appfile{}
		comp := &common.ApplicationComponent{
			Type: v1alpha1.RefObjectsComponentType,
			Properties: &runtime.RawExtension{
				Raw: []byte(`{"objects": "not-an-array"}`),
			},
		}
		_, err := af.LoadDynamicComponent(ctx, cli, comp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid ref-objects component properties")
	})

	t.Run("Success with URL selectors", func(t *testing.T) {
		objWithURL := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"name":      "secret1",
					"namespace": "default",
					"annotations": map[string]interface{}{
						oam.AnnotationResourceURL: "http://example.com/secret.yaml",
					},
				},
			},
		}
		af := &Appfile{
			Namespace:       "default",
			ReferredObjects: []*unstructured.Unstructured{objWithURL},
			app:             &v1beta1.Application{},
		}
		comp := &common.ApplicationComponent{
			Name: "my-comp",
			Type: v1alpha1.RefObjectsComponentType,
			Properties: &runtime.RawExtension{
				Raw: []byte(`{"urls": ["http://example.com/secret.yaml"]}`),
			},
		}

		resultComp, err := af.LoadDynamicComponent(ctx, cli, comp)
		assert.NoError(t, err)

		refObjList := &common.ReferredObjectList{}
		err = json.Unmarshal(resultComp.Properties.Raw, refObjList)
		assert.NoError(t, err)
		assert.Len(t, refObjList.Objects, 1)

		u := &unstructured.Unstructured{}
		err = u.UnmarshalJSON(refObjList.Objects[0].Raw)
		assert.NoError(t, err)
		assert.Equal(t, "Secret", u.GetKind())
		assert.Equal(t, "secret1", u.GetName())
	})
}

func TestPolicyClient(t *testing.T) {
	ctx := context.Background()
	policy := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "my-policy", Namespace: "default"},
		Type:       "override",
	}
	policyKey := client.ObjectKeyFromObject(policy)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	cmKey := client.ObjectKeyFromObject(cm)

	t.Run("With AppRevision, policy in cache", func(t *testing.T) {
		af := &Appfile{
			AppRevision: &v1beta1.ApplicationRevision{},
			ExternalPolicies: map[string]*v1alpha1.Policy{
				policyKey.String(): policy,
			},
		}
		cli := fake.NewClientBuilder().Build()
		policyClient := af.PolicyClient(cli)

		retrievedPolicy := &v1alpha1.Policy{}
		err := policyClient.Get(ctx, policyKey, retrievedPolicy)
		assert.NoError(t, err)
		assert.Equal(t, policy, retrievedPolicy)
	})

	t.Run("With AppRevision, policy not in cache", func(t *testing.T) {
		af := &Appfile{
			AppRevision:      &v1beta1.ApplicationRevision{},
			ExternalPolicies: map[string]*v1alpha1.Policy{},
		}
		// The policy exists in the client, but it should not be returned
		cli := fake.NewClientBuilder().WithObjects(policy).Build()
		policyClient := af.PolicyClient(cli)

		retrievedPolicy := &v1alpha1.Policy{}
		err := policyClient.Get(ctx, policyKey, retrievedPolicy)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("With AppRevision, get non-policy object", func(t *testing.T) {
		af := &Appfile{
			AppRevision: &v1beta1.ApplicationRevision{},
		}
		cli := fake.NewClientBuilder().WithObjects(cm).Build()
		policyClient := af.PolicyClient(cli)

		retrievedCm := &corev1.ConfigMap{}
		err := policyClient.Get(ctx, cmKey, retrievedCm)
		assert.NoError(t, err)
		assert.Equal(t, cm, retrievedCm)
	})

	t.Run("Without AppRevision, policy in client", func(t *testing.T) {
		af := &Appfile{
			ExternalPolicies: make(map[string]*v1alpha1.Policy),
		}
		cli := fake.NewClientBuilder().WithObjects(policy).Build()
		policyClient := af.PolicyClient(cli)

		retrievedPolicy := &v1alpha1.Policy{}
		err := policyClient.Get(ctx, policyKey, retrievedPolicy)
		assert.NoError(t, err)
		assert.Equal(t, policy, retrievedPolicy)

		// Check if the policy is now cached
		cachedPolicy, found := af.ExternalPolicies[policyKey.String()]
		assert.True(t, found)
		assert.Equal(t, policy, cachedPolicy)
	})

	t.Run("Without AppRevision, policy not in client", func(t *testing.T) {
		af := &Appfile{
			ExternalPolicies: make(map[string]*v1alpha1.Policy),
		}
		cli := fake.NewClientBuilder().Build()
		policyClient := af.PolicyClient(cli)

		retrievedPolicy := &v1alpha1.Policy{}
		err := policyClient.Get(ctx, policyKey, retrievedPolicy)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("Without AppRevision, get non-policy object", func(t *testing.T) {
		af := &Appfile{}
		cli := fake.NewClientBuilder().WithObjects(cm).Build()
		policyClient := af.PolicyClient(cli)

		retrievedCm := &corev1.ConfigMap{}
		err := policyClient.Get(ctx, cmKey, retrievedCm)
		assert.NoError(t, err)
		assert.Equal(t, cm, retrievedCm)
	})
}

func TestWorkflowClient(t *testing.T) {
	ctx := context.Background()
	workflow := &workflowv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "my-workflow", Namespace: "default"},
	}
	workflowKey := client.ObjectKeyFromObject(workflow)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	cmKey := client.ObjectKeyFromObject(cm)

	t.Run("With AppRevision, workflow in cache", func(t *testing.T) {
		af := &Appfile{
			AppRevision:      &v1beta1.ApplicationRevision{},
			ExternalWorkflow: workflow,
		}
		cli := fake.NewClientBuilder().Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedWorkflow := &workflowv1alpha1.Workflow{}
		err := workflowClient.Get(ctx, workflowKey, retrievedWorkflow)
		assert.NoError(t, err)
		assert.Equal(t, workflow, retrievedWorkflow)
	})

	t.Run("With AppRevision, workflow not in cache", func(t *testing.T) {
		af := &Appfile{
			AppRevision: &v1beta1.ApplicationRevision{},
		}
		// The workflow exists in the client, but it should not be returned
		cli := fake.NewClientBuilder().WithObjects(workflow).Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedWorkflow := &workflowv1alpha1.Workflow{}
		err := workflowClient.Get(ctx, workflowKey, retrievedWorkflow)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("With AppRevision, get non-workflow object", func(t *testing.T) {
		af := &Appfile{
			AppRevision: &v1beta1.ApplicationRevision{},
		}
		cli := fake.NewClientBuilder().WithObjects(cm).Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedCm := &corev1.ConfigMap{}
		err := workflowClient.Get(ctx, cmKey, retrievedCm)
		assert.NoError(t, err)
		assert.Equal(t, cm, retrievedCm)
	})

	t.Run("Without AppRevision, workflow in client", func(t *testing.T) {
		af := &Appfile{}
		cli := fake.NewClientBuilder().WithObjects(workflow).Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedWorkflow := &workflowv1alpha1.Workflow{}
		err := workflowClient.Get(ctx, workflowKey, retrievedWorkflow)
		assert.NoError(t, err)
		assert.Equal(t, workflow, retrievedWorkflow)

		// Check if the workflow is now cached
		assert.Equal(t, workflow, af.ExternalWorkflow)
	})

	t.Run("Without AppRevision, workflow not in client", func(t *testing.T) {
		af := &Appfile{}
		cli := fake.NewClientBuilder().Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedWorkflow := &workflowv1alpha1.Workflow{}
		err := workflowClient.Get(ctx, workflowKey, retrievedWorkflow)
		assert.Error(t, err)
		assert.True(t, kerrors.IsNotFound(err))
	})

	t.Run("Without AppRevision, get non-workflow object", func(t *testing.T) {
		af := &Appfile{}
		cli := fake.NewClientBuilder().WithObjects(cm).Build()
		workflowClient := af.WorkflowClient(cli)

		retrievedCm := &corev1.ConfigMap{}
		err := workflowClient.Get(ctx, cmKey, retrievedCm)
		assert.NoError(t, err)
		assert.Equal(t, cm, retrievedCm)
	})
}

func TestSetWorkloadRefToTrait(t *testing.T) {
	wlRef := corev1.ObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "my-workload",
	}

	tests := map[string]struct {
		reason    string
		appfile   *Appfile
		trait     *unstructured.Unstructured
		wantTrait *unstructured.Unstructured
		wantErr   error
	}{
		"AuxiliaryWorkloadTrait": {
			reason:  "Should do nothing for auxiliary-workload traits",
			appfile: &Appfile{},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: definition.AuxiliaryWorkload,
					},
				},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: definition.AuxiliaryWorkload,
					},
				},
			}},
			wantErr: nil,
		},
		"TraitDefinitionNotFound": {
			reason: "Should return error if TraitDefinition is not found",
			appfile: &Appfile{
				RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{},
			},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantErr: errors.Errorf("TraitDefinition %s not found in appfile", "my-trait"),
		},
		"WorkloadRefPathIsEmpty": {
			reason: "Should do nothing if workloadRefPath is empty",
			appfile: &Appfile{
				RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"my-trait": {
						Spec: v1beta1.TraitDefinitionSpec{
							WorkloadRefPath: "",
						},
					},
				},
			},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantErr: nil,
		},
		"SetWorkloadRefSucceeds": {
			reason: "Should set workload reference correctly",
			appfile: &Appfile{
				RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"my-trait": {
						Spec: v1beta1.TraitDefinitionSpec{
							WorkloadRefPath: "spec.workloadRef",
						},
					},
				},
			},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
				"spec": map[string]interface{}{},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
				"spec": map[string]interface{}{
					"workloadRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-workload",
					},
				},
			}},
			wantErr: nil,
		},
		"SetWorkloadRefWithSuffixedTraitType": {
			reason: "Should find the base trait definition and set workload reference",
			appfile: &Appfile{
				RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"my-trait": {
						Spec: v1beta1.TraitDefinitionSpec{
							WorkloadRefPath: "spec.workloadRef",
						},
					},
				},
			},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait-abcde",
					},
				},
				"spec": map[string]interface{}{},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait-abcde",
					},
				},
				"spec": map[string]interface{}{
					"workloadRef": map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "my-workload",
					},
				},
			}},
			wantErr: nil,
		},
		"InvalidWorkloadRefPath": {
			reason: "Should return an error for an invalid workloadRefPath",
			appfile: &Appfile{
				RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
					"my-trait": {
						Spec: v1beta1.TraitDefinitionSpec{
							WorkloadRefPath: "spec[workloadRef", // Invalid path
						},
					},
				},
			},
			trait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantTrait: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						oam.TraitTypeLabel: "my-trait",
					},
				},
			}},
			wantErr: fieldpath.Pave(map[string]interface{}{}).SetValue("spec[workloadRef", wlRef),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			traitCopy := tc.trait.DeepCopy()
			err := tc.appfile.setWorkloadRefToTrait(wlRef, traitCopy)

			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nsetWorkloadRefToTrait(...): -want error, +got error:\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantTrait, traitCopy); diff != "" {
				t.Errorf("\n%s\nsetWorkloadRefToTrait(...): -want trait, +got trait:\n%s\n", tc.reason, diff)
			}
		})
	}
}

func TestSetOAMContract(t *testing.T) {
	baseAppfile := &Appfile{
		Name:            "test-app",
		Namespace:       "test-ns",
		AppRevisionName: "test-app-v1",
		AppLabels: map[string]string{
			"app-label": "app-label-val",
		},
		AppAnnotations: map[string]string{
			"app-annot": "app-annot-val",
		},
		RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
			"my-trait": {
				Spec: v1beta1.TraitDefinitionSpec{
					WorkloadRefPath: "spec.workloadRef",
				},
			},
			"no-ref-trait": {
				Spec: v1beta1.TraitDefinitionSpec{},
			},
		},
	}

	baseWorkload := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "my-workload",
			},
		},
	}

	baseTrait := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{
					oam.TraitTypeLabel: "my-trait",
				},
			},
		},
	}

	tests := map[string]struct {
		reason   string
		appfile  *Appfile
		comp     *oamtypes.ComponentManifest
		wantComp *oamtypes.ComponentManifest
		wantErr  bool
	}{
		"SuccessBasic": {
			reason:  "A basic workload and trait should be assembled correctly",
			appfile: baseAppfile,
			comp: &oamtypes.ComponentManifest{
				Name:            "my-comp",
				ComponentOutput: baseWorkload.DeepCopy(),
				ComponentOutputsAndTraits: []*unstructured.Unstructured{
					baseTrait.DeepCopy(),
				},
			},
			wantComp: &oamtypes.ComponentManifest{
				Name: "my-comp",
				ComponentOutput: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata": map[string]interface{}{
							"name":      "my-workload",
							"namespace": "test-ns",
							"labels": map[string]interface{}{
								"app-label":              "app-label-val",
								oam.LabelAppName:         "test-app",
								oam.LabelAppNamespace:    "test-ns",
								oam.LabelAppRevision:     "test-app-v1",
								oam.LabelAppComponent:    "my-comp",
								oam.LabelOAMResourceType: oam.ResourceTypeWorkload,
							},
							"annotations": map[string]interface{}{
								"app-annot": "app-annot-val",
							},
						},
					},
				},
				ComponentOutputsAndTraits: []*unstructured.Unstructured{
					{
						Object: map[string]interface{}{
							"apiVersion": "v1",
							"kind":       "Service",
							"metadata": map[string]interface{}{
								"name":      "", // Name will be generated
								"namespace": "test-ns",
								"labels": map[string]interface{}{
									"app-label":              "app-label-val",
									oam.TraitTypeLabel:       "my-trait",
									oam.LabelAppName:         "test-app",
									oam.LabelAppNamespace:    "test-ns",
									oam.LabelAppRevision:     "test-app-v1",
									oam.LabelAppComponent:    "my-comp",
									oam.LabelOAMResourceType: oam.ResourceTypeTrait,
								},
								"annotations": map[string]interface{}{
									"app-annot": "app-annot-val",
								},
							},
							"spec": map[string]interface{}{
								"workloadRef": map[string]interface{}{
									"apiVersion": "apps/v1",
									"kind":       "Deployment",
									"name":       "my-workload",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			compCopy := deepCopyComponentManifest(tc.comp)
			err := tc.appfile.SetOAMContract(compCopy)

			if (err != nil) != tc.wantErr {
				t.Errorf("\n%s\nSetOAMContract(...): want error? %v, got error: %v\n", tc.reason, tc.wantErr, err)
			}

			if name == "SuccessBasic" || name == "SuccessNameGeneration" {
				for i, trait := range compCopy.ComponentOutputsAndTraits {
					gotName := trait.GetName()
					wantTrait := tc.wantComp.ComponentOutputsAndTraits[i]
					traitType := trait.GetLabels()[oam.TraitTypeLabel]
					expectedPrefix := compCopy.Name + "-" + traitType + "-"

					if !strings.HasPrefix(gotName, expectedPrefix) {
						t.Errorf("\n%s\nSetOAMContract(...): trait name %q does not have expected prefix %q", tc.reason, gotName, expectedPrefix)
					}
					trait.SetName("")
					wantTrait.SetName("")
				}
			}

			if !tc.wantErr {
				if diff := cmp.Diff(tc.wantComp.ComponentOutput, compCopy.ComponentOutput); diff != "" {
					t.Errorf("\n%s\nSetOAMContract(...): -want workload, +got workload:\n%s\n", tc.reason, diff)
				}
				if diff := cmp.Diff(tc.wantComp.ComponentOutputsAndTraits, compCopy.ComponentOutputsAndTraits); diff != "" {
					t.Errorf("\n%s\nSetOAMContract(...): -want traits, +got traits:\n%s\n", tc.reason, diff)
				}
			}
		})
	}
}

func deepCopyComponentManifest(in *oamtypes.ComponentManifest) *oamtypes.ComponentManifest {
	if in == nil {
		return nil
	}
	out := &oamtypes.ComponentManifest{
		Name: in.Name,
	}
	if in.ComponentOutput != nil {
		out.ComponentOutput = in.ComponentOutput.DeepCopy()
	}
	if in.ComponentOutputsAndTraits != nil {
		out.ComponentOutputsAndTraits = make([]*unstructured.Unstructured, len(in.ComponentOutputsAndTraits))
		for i, trait := range in.ComponentOutputsAndTraits {
			if trait != nil {
				out.ComponentOutputsAndTraits[i] = trait.DeepCopy()
			}
		}
	}
	return out
}

func TestGenerateComponentManifests(t *testing.T) {
	baseEngine := definition.NewWorkloadAbstractEngine("test-engine")
	baseTraitEngine := definition.NewTraitAbstractEngine("scaler")
	baseTemplate := &Template{
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
						spec: containers: [{
							name:  context.name
							image: parameter.image
						}]
					}
				}
			}
			parameter: {
				image: string
			}`,
	}
	baseTraitTemplate := &Trait{
		Name:   "scaler",
		engine: baseTraitEngine,
		Template: `
			outputs: scaler: {
				apiVersion: "autoscaling/v2beta2"
				kind:       "HorizontalPodAutoscaler"
				spec: {
					scaleTargetRef: {
						apiVersion: "apps/v1"
						kind:       "Deployment"
						name:       context.name
					}
					minReplicas: parameter.min
					maxReplicas: parameter.max
				}
			}
			parameter: {
				min: *1 | int
				max: *10 | int
			}
		`,
		Params: map[string]interface{}{
			"min": 2,
			"max": 15,
		},
	}

	type fields struct {
		Appfile *Appfile
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Test generate component manifests",
			fields: fields{
				Appfile: &Appfile{
					Name:            "test-app",
					Namespace:       "test-ns",
					AppRevisionName: "test-app-v1",
					ParsedComponents: []*Component{
						{
							Name:   "test-comp",
							Type:   "worker",
							Params: map[string]interface{}{"image": "nginx:1.10"},
							engine: baseEngine,
							FullTemplate: &Template{
								TemplateStr: baseTemplate.TemplateStr,
							},
							Traits: []*Trait{baseTraitTemplate},
						},
					},
					RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
						"scaler": {
							Spec: v1beta1.TraitDefinitionSpec{
								WorkloadRefPath: "spec.scaleTargetRef",
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af := tt.fields.Appfile
			got, err := af.GenerateComponentManifests()
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateComponentManifests() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, len(af.ParsedComponents), len(got))
			assert.Equal(t, "test-comp", got[0].Name)
			assert.Equal(t, "Deployment", got[0].ComponentOutput.GetKind())
			assert.Equal(t, "test-comp", got[0].ComponentOutput.GetName())
			assert.Equal(t, "test-ns", got[0].ComponentOutput.GetNamespace())
			assert.Equal(t, "test-app", got[0].ComponentOutput.GetLabels()[oam.LabelAppName])
			assert.Equal(t, 1, len(got[0].ComponentOutputsAndTraits))
			trait := got[0].ComponentOutputsAndTraits[0]
			assert.Equal(t, "HorizontalPodAutoscaler", trait.GetKind())
			assert.True(t, strings.HasPrefix(trait.GetName(), "test-comp-scaler-"))
			paved := fieldpath.Pave(trait.Object)
			name, err := paved.GetString("spec.scaleTargetRef.name")
			assert.NoError(t, err)
			assert.Equal(t, "test-comp", name)
			assert.Equal(t, af.Artifacts, got)
		})
	}
}

func TestGeneratePolicyManifests(t *testing.T) {
	policyEngine := definition.NewWorkloadAbstractEngine("test-policy")
	policyTemplate := &Template{
		TemplateStr: `
			output: {
				apiVersion: "v1"
				kind:       "ConfigMap"
				metadata: {
					name: context.name
				}
				data: {
					key: parameter.value
				}
			}
			parameter: {
				value: string
			}
		`,
	}

	af := &Appfile{
		Name:            "test-app",
		Namespace:       "test-ns",
		AppRevisionName: "test-app-v1",
		ParsedPolicies: []*Component{
			{
				Name:   "test-policy",
				Type:   "override",
				Params: map[string]interface{}{"value": "test-value"},
				engine: policyEngine,
				FullTemplate: &Template{
					TemplateStr: policyTemplate.TemplateStr,
				},
			},
		},
		AppLabels: map[string]string{
			"label-key": "label-value",
		},
	}

	manifests, err := af.GeneratePolicyManifests(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(manifests))

	cm := manifests[0]
	assert.Equal(t, "v1", cm.GetAPIVersion())
	assert.Equal(t, "ConfigMap", cm.GetKind())
	assert.Equal(t, "test-policy", cm.GetName())
	assert.Equal(t, "test-ns", cm.GetNamespace())
	assert.Equal(t, "test-app", cm.GetLabels()[oam.LabelAppName])

	data, found, err := unstructured.NestedStringMap(cm.Object, "data")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test-value", data["key"])
}
