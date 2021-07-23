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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Helm schematic appfile", func() {
	var (
		appName  = "test-app"
		compName = "test-comp"
	)

	It("Test generate AppConfig resources from Helm schematic", func() {
		appFile := &Appfile{
			Name:         appName,
			Namespace:    "default",
			RevisionName: appName + "-v1",
			Workloads: []*Workload{
				{
					Name:               compName,
					Type:               "webapp-chart",
					CapabilityCategory: oamtypes.HelmCategory,
					Params: map[string]interface{}{
						"image": map[string]interface{}{
							"tag": "5.1.2",
						},
					},
					engine: definition.NewWorkloadAbstractEngine(compName, pd),
					Traits: []*Trait{
						{
							Name: "scaler",
							Params: map[string]interface{}{
								"replicas": float64(10),
							},
							engine: definition.NewTraitAbstractEngine("scaler", pd),
							Template: `
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }
`,
						},
					},
					FullTemplate: &Template{
						Reference: common.WorkloadTypeDescriptor{
							Definition: common.WorkloadGVK{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
							},
						},
						Helm: &common.Helm{
							Release: util.Object2RawExtension(map[string]interface{}{
								"chart": map[string]interface{}{
									"spec": map[string]interface{}{
										"chart":   "podinfo",
										"version": "5.1.4",
									},
								},
							}),
							Repository: util.Object2RawExtension(map[string]interface{}{
								"url": "http://oam.dev/catalog/",
							}),
						},
					},
				},
			},
		}
		By("Generate ApplicationConfiguration and Components")
		components, err := appFile.GenerateComponentManifests()
		Expect(err).To(BeNil())

		expectCompManifest := &oamtypes.ComponentManifest{
			Name: compName,
			StandardWorkload: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"workload.oam.dev/type":   "webapp-chart",
							"app.oam.dev/component":   compName,
							"app.oam.dev/name":        appName,
							"app.oam.dev/appRevision": appName + "-v1",
						}}}},
			Traits: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "core.oam.dev/v1alpha2",
						"kind":       "ManualScalerTrait",
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app.oam.dev/component":   compName,
								"app.oam.dev/name":        appName,
								"trait.oam.dev/type":      "scaler",
								"trait.oam.dev/resource":  "scaler",
								"app.oam.dev/appRevision": appName + "-v1",
							},
						},
						"spec": map[string]interface{}{"replicaCount": int64(10)},
					},
				},
			},
			PackagedWorkloadResources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "helm.toolkit.fluxcd.io/v2beta1",
						"kind":       "HelmRelease",
						"metadata": map[string]interface{}{
							"name":      fmt.Sprintf("%s-%s", appName, compName),
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"chart": map[string]interface{}{
								"spec": map[string]interface{}{
									"sourceRef": map[string]interface{}{
										"kind":      "HelmRepository",
										"name":      fmt.Sprintf("%s-%s", appName, compName),
										"namespace": "default",
									},
								},
							},
							"interval": "5m0s",
							"values": map[string]interface{}{
								"image": map[string]interface{}{
									"tag": "5.1.2",
								},
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"apiVersion": "source.toolkit.fluxcd.io/v1beta1",
						"kind":       "HelmRepository",
						"metadata": map[string]interface{}{
							"name":      fmt.Sprintf("%s-%s", appName, compName),
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"url": "http://oam.dev/catalog/",
						},
					},
				},
			},
		}
		By("Verify expected ComponentManifest")
		diff := cmp.Diff(components[0], expectCompManifest)
		Expect(diff).ShouldNot(BeEmpty())
	})

})

var _ = Describe("Test Kube schematic appfile", func() {
	var (
		appName  = "test-app"
		compName = "test-comp"
	)
	var testTemplate = func() runtime.RawExtension {
		yamlStr := `apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
      ports:
      - containerPort: 80 `
		b, _ := yaml.YAMLToJSON([]byte(yamlStr))
		return runtime.RawExtension{Raw: b}
	}

	var testAppfile = func() *Appfile {
		return &Appfile{
			RevisionName: appName + "-v1",
			Name:         appName,
			Namespace:    "default",
			Workloads: []*Workload{
				{
					Name:               compName,
					Type:               "kube-worker",
					CapabilityCategory: oamtypes.KubeCategory,
					Params: map[string]interface{}{
						"image": "nginx:1.14.0",
					},
					engine: definition.NewWorkloadAbstractEngine(compName, pd),
					Traits: []*Trait{
						{
							Name: "scaler",
							Params: map[string]interface{}{
								"replicas": float64(10),
							},
							engine: definition.NewTraitAbstractEngine("scaler", pd),
							Template: `
      outputs: scaler: {
      	apiVersion: "core.oam.dev/v1alpha2"
      	kind:       "ManualScalerTrait"
      	spec: {
      		replicaCount: parameter.replicas
      	}
      }
      parameter: {
      	//+short=r
      	replicas: *1 | int
      }
`,
						},
					},
					FullTemplate: &Template{
						Kube: &common.Kube{
							Template: testTemplate(),
							Parameters: []common.KubeParameter{
								{
									Name:       "image",
									ValueType:  common.StringType,
									Required:   pointer.BoolPtr(true),
									FieldPaths: []string{"spec.template.spec.containers[0].image"},
								},
							},
						},
						Reference: common.WorkloadTypeDescriptor{
							Definition: common.WorkloadGVK{
								APIVersion: "apps/v1",
								Kind:       "Deployment",
							},
						},
					},
				},
			},
		}

	}

	It("Test generate AppConfig resources from Kube schematic", func() {
		By("Generate ApplicationConfiguration and Components")
		comps, err := testAppfile().GenerateComponentManifests()
		Expect(err).To(BeNil())

		expectWorkload := func() *unstructured.Unstructured {
			yamlStr := `apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.14.0
      ports:
      - containerPort: 80 `
			r := &unstructured.Unstructured{}
			_ = yaml.Unmarshal([]byte(yamlStr), r)
			return r
		}()

		expectCompManifest := &oamtypes.ComponentManifest{
			Name:             compName,
			StandardWorkload: expectWorkload,
			Traits: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "core.oam.dev/v1alpha2",
						"kind":       "ManualScalerTrait",
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{
								"app.oam.dev/component":   compName,
								"app.oam.dev/name":        appName,
								"app.oam.dev/appRevision": appName + "-v1",
								"trait.oam.dev/type":      "scaler",
								"trait.oam.dev/resource":  "scaler",
							},
						},
						"spec": map[string]interface{}{"replicaCount": int64(10)},
					},
				},
			},
		}
		By("Verify expected Component")
		diff := cmp.Diff(comps[0], expectCompManifest)
		Expect(diff).ShouldNot(BeEmpty())
	})

	It("Test missing set required parameter", func() {
		appfile := testAppfile()
		// remove parameter settings
		appfile.Workloads[0].Params = nil
		_, err := appfile.GenerateComponentManifests()

		expectError := errors.WithMessage(errors.New(`require parameter "image"`), "cannot resolve parameter settings")
		diff := cmp.Diff(expectError, err, test.EquateErrors())
		Expect(diff).Should(BeEmpty())
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

		wl := &Workload{
			Name: "sample-db",
			FullTemplate: &Template{
				Terraform: &common.Terraform{
					Configuration: configuration,
					Type:          "hcl",
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
			Workloads:    []*Workload{wl},
			Name:         appName,
			RevisionName: revision,
			Namespace:    ns,
		}

		variable := map[string]interface{}{"account_name": "oamtest"}
		data, _ := json.Marshal(variable)
		raw := &runtime.RawExtension{}
		raw.Raw = data

		workload := terraformapi.Configuration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "terraform.core.oam.dev/v1beta1",
				Kind:       "Configuration",
			},
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
				HCL:                              configuration,
				Variable:                         raw,
				WriteConnectionSecretToReference: &terraformtypes.SecretReference{Name: "db", Namespace: "default"},
			},
			Status: terraformapi.ConfigurationStatus{},
		}

		expectCompManifest := &oamtypes.ComponentManifest{
			Name: compName,
			StandardWorkload: func() *unstructured.Unstructured {
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

func TestResolveKubeParameters(t *testing.T) {
	stringParam := &common.KubeParameter{
		Name:       "strParam",
		ValueType:  common.StringType,
		FieldPaths: []string{"spec"},
	}
	requiredParam := &common.KubeParameter{
		Name:       "reqParam",
		Required:   pointer.BoolPtr(true),
		ValueType:  common.StringType,
		FieldPaths: []string{"spec"},
	}
	tests := map[string]struct {
		reason   string
		params   []common.KubeParameter
		settings map[string]interface{}
		want     paramValueSettings
		wantErr  error
	}{
		"EmptyParam": {
			reason: "Empty value settings and no error should be returned",
			want:   make(paramValueSettings),
		},
		"UnsupportedParam": {
			reason:   "An error shoulde be returned because of unsupported param",
			params:   []common.KubeParameter{*stringParam},
			settings: map[string]interface{}{"unsupported": "invalid parameter"},
			want:     nil,
			wantErr:  errors.Errorf("unsupported parameter %q", "unsupported"),
		},
		"MissingRequiredParam": {
			reason:   "An error should be returned because of missing required param",
			params:   []common.KubeParameter{*stringParam, *requiredParam},
			settings: map[string]interface{}{"strParam": "string"},
			want:     nil,
			wantErr:  errors.Errorf("require parameter %q", "reqParam"),
		},
		"Succeed": {
			reason:   "No error should be returned",
			params:   []common.KubeParameter{*stringParam, *requiredParam},
			settings: map[string]interface{}{"strParam": "test", "reqParam": "test"},
			want: paramValueSettings{
				"strParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: stringParam.FieldPaths,
				},
				"reqParam": paramValueSetting{
					Value:      "test",
					ValueType:  common.StringType,
					FieldPaths: requiredParam.FieldPaths,
				},
			},
			wantErr: nil,
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			result, err := resolveKubeParameters(tc.params, tc.settings)
			if diff := cmp.Diff(tc.want, result); diff != "" {
				t.Fatalf("\nresolveKubeParameters(...)(...) -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.wantErr, err, test.EquateErrors()); diff != "" {
				t.Fatalf("\nresolveKubeParameters(...)(...) -want +get \nreason:%s\n%s\n", tc.reason, diff)
			}
		})
	}

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
			wl       *Workload
			appName  string
			revision string
		}

		args := appArgs{
			wl: &Workload{
				Name: "sample-db",
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

		pCtx := NewBasicContext(args.wl, args.appName, args.revision, ns)
		comp, err := evalWorkloadWithContext(pCtx, args.wl, ns, args.appName, compName)
		Expect(comp.StandardWorkload).ShouldNot(BeNil())
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
		json                       string
		hcl                        string
		params                     map[string]interface{}
	}

	type want struct {
		err error
	}

	testcases := map[string]struct {
		args args
		want want
	}{
		"json workload with invalid secret": {
			args: args{
				json:   "abc",
				params: map[string]interface{}{"acl": "private", "writeConnectionSecretToRef": map[string]interface{}{"name": "", "namespace": ""}},
			},
			want: want{err: errors.New(errTerraformNameOfWriteConnectionSecretToRefNotSet)}},

		"json workload with secret": {
			args: args{

				json: "abc",
				params: map[string]interface{}{"acl": "private",
					"writeConnectionSecretToRef": map[string]interface{}{"name": "oss", "namespace": ""}},
				writeConnectionSecretToRef: &terraformtypes.SecretReference{Name: "oss", Namespace: "default"},
			},
			want: want{err: nil}},

		"valid hcl workload": {
			args: args{
				hcl: "abc",
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
			want: want{err: errors.Wrap(badParamMarshalError, errFailToConvertTerraformComponentProperties)}},
	}

	for tcName, tc := range testcases {
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
				HCL:                              tc.args.hcl,
				Variable:                         raw,
				WriteConnectionSecretToReference: tc.args.writeConnectionSecretToRef,
			}
		}
		if tc.args.json != "" {
			template = &Template{
				Terraform: &common.Terraform{
					Configuration: tc.args.json,
					Type:          "json",
				},
			}
			configSpec = terraformapi.ConfigurationSpec{
				JSON:                             tc.args.json,
				Variable:                         raw,
				WriteConnectionSecretToReference: tc.args.writeConnectionSecretToRef,
			}
		}
		if tc.args.hcl == "" && tc.args.json == "" {
			template = &Template{
				Terraform: &common.Terraform{},
			}

			configSpec = terraformapi.ConfigurationSpec{
				Variable:                         raw,
				WriteConnectionSecretToReference: tc.args.writeConnectionSecretToRef,
			}
		}

		wl := &Workload{
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
				TypeMeta:   metav1.TypeMeta{APIVersion: "terraform.core.oam.dev/v1beta1", Kind: "Configuration"},
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
				Spec:       configSpec,
			}
			rawConf := util.Object2RawExtension(tfConfiguration)
			wantWL, _ := util.RawExtension2Unstructured(&rawConf)

			if diff := cmp.Diff(wantWL, got); diff != "" {
				t.Errorf("\n%s\ngenerateTerraformConfigurationWorkload(...): -want, +got:\n%s\n", tcName, diff)
			}
		}
	}
}

func TestGetUserConfigName(t *testing.T) {
	wl1 := &Workload{Params: nil}
	assert.Equal(t, wl1.GetUserConfigName(), "")

	wl2 := &Workload{Params: map[string]interface{}{AppfileBuiltinConfig: 1}}
	assert.Equal(t, wl2.GetUserConfigName(), "")

	config := "abc"
	wl3 := &Workload{Params: map[string]interface{}{AppfileBuiltinConfig: config}}
	assert.Equal(t, wl3.GetUserConfigName(), config)
}

func TestGenerateCUETemplate(t *testing.T) {

	var testCorrectTemplate = func() runtime.RawExtension {
		yamlStr := `apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
      ports:
      - containerPort: 80 `
		b, _ := yaml.YAMLToJSON([]byte(yamlStr))
		return runtime.RawExtension{Raw: b}
	}

	var testErrorTemplate = func() runtime.RawExtension {
		yamlStr := `apiVersion: apps/v1
kind: Deployment
spec:
  template:
	selector:
		matchLabels:
		app: nginx
`
		b, _ := yaml.YAMLToJSON([]byte(yamlStr))
		return runtime.RawExtension{Raw: b}
	}

	testcases := map[string]struct {
		workload   *Workload
		expectData string
		hasError   bool
		errInfo    string
	}{"Kube workload with Correct template": {
		workload: &Workload{
			FullTemplate: &Template{
				Kube: &common.Kube{
					Template: testCorrectTemplate(),
					Parameters: []common.KubeParameter{
						{
							Name:       "image",
							ValueType:  common.StringType,
							Required:   pointer.BoolPtr(true),
							FieldPaths: []string{"spec.template.spec.containers[0].image"},
						},
					},
				},
				Reference: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			Params: map[string]interface{}{
				"image": "nginx:1.14.0",
			},
			CapabilityCategory: oamtypes.KubeCategory,
		},
		expectData: `
output: { 
apiVersion: "apps/v1"
kind:       "Deployment"
spec: {
	selector: {
		matchLabels: {
			app: "nginx"
		}
	}
	template: {
		spec: {
			containers: [{
				name:  "nginx"
				image: "nginx:1.14.0"
			}]
			ports: [{
				containerPort: 80
			}]
		}
		metadata: {
			labels: {
				app: "nginx"
			}
		}
	}
}
 
}`,
		hasError: false,
	}, "Kube workload with wrong template": {
		workload: &Workload{
			FullTemplate: &Template{
				Kube: &common.Kube{
					Template: testErrorTemplate(),
					Parameters: []common.KubeParameter{
						{
							Name:       "image",
							ValueType:  common.StringType,
							Required:   pointer.BoolPtr(true),
							FieldPaths: []string{"spec.template.spec.containers[0].image"},
						},
					},
				},
				Reference: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			Params: map[string]interface{}{
				"image": "nginx:1.14.0",
			},
			CapabilityCategory: oamtypes.KubeCategory,
		},
		hasError: true,
		errInfo:  "cannot decode Kube template into K8s object: unexpected end of JSON input",
	}, "Kube workload with wrong parameter": {
		workload: &Workload{
			FullTemplate: &Template{
				Kube: &common.Kube{
					Template: testCorrectTemplate(),
					Parameters: []common.KubeParameter{
						{
							Name:       "image",
							ValueType:  common.StringType,
							Required:   pointer.BoolPtr(true),
							FieldPaths: []string{"spec.template.spec.containers[0].image"},
						},
					},
				},
				Reference: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
			Params: map[string]interface{}{
				"unsupported": "invalid parameter",
			},
			CapabilityCategory: oamtypes.KubeCategory,
		},
		hasError: true,
		errInfo:  "cannot resolve parameter settings: unsupported parameter \"unsupported\"",
	}, "Helm workload with correct reference": {
		workload: &Workload{
			FullTemplate: &Template{
				Reference: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "app/v1",
						Kind:       "deployment",
					},
				},
			},
			CapabilityCategory: oamtypes.HelmCategory,
		},
		hasError: false,
		expectData: `
output: {
	apiVersion: "app/v1"
	kind: "deployment"
}`,
	}, "Helm workload with wrong reference": {
		workload: &Workload{
			FullTemplate: &Template{
				Reference: common.WorkloadTypeDescriptor{
					Definition: common.WorkloadGVK{
						APIVersion: "app@//v1",
						Kind:       "deployment",
					},
				},
			},
			CapabilityCategory: oamtypes.HelmCategory,
		},
		hasError: true,
		errInfo:  "unexpected GroupVersion string: app@//v1",
	}}

	for _, tc := range testcases {
		template, err := GenerateCUETemplate(tc.workload)
		assert.Equal(t, err != nil, tc.hasError)
		if tc.hasError {
			assert.Equal(t, tc.errInfo, err.Error())
			continue
		}
		assert.Equal(t, tc.expectData, template)
	}
}

func TestGetSecretAndConfigs(t *testing.T) {
	secretData := map[string][]byte{
		"username": []byte("test-name"),
		"password": []byte("test-pwd"),
	}

	secretConsumerTemplate := `{
	apiVersion: "apps/v1"
	kind:       "Deployment"
	spec: {
		selector: matchLabels: {
			"app.oam.dev/component": "test"
		}
		template: {
			metadata: labels: {
				"app.oam.dev/component": "test"
			}
			spec: {
				containers: [{
					name:  "test"
					if parameter["dbSecret"] != _|_ {
						env: [
							{
								name:  "username"
								value: dbConn.username
							},
							{
								name:  "DB_PASSWORD"
								value: dbConn.password
							},
						]
					}
				}]
			}
		}
	}
}
parameter: {
	// +usage=Referred db secret
	// +insertSecretTo=dbConn
	dbSecret?: string
}

dbConn: {
	username: string
	password: string
}
`
	userConfigTemplate := `
output: {
  apiVersion: "apps/v1"
  kind:       "Deployment"
  metadata: {
	  annotations: {
		  if context["config"] != _|_ {
			  for _, v in context.config {
				  "\(v.name)" : v.value
			  }
		  }
	  }
  }
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
  }
}

parameter: {
  // +usage=Which image would you like to use for your service
  // +short=i
  image: string

  cmd?: [...string]
}
`

	mockSecretClient := func(data map[string][]byte) *test.MockClient {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
				switch secret := obj.(type) {
				case *v1.Secret:
					t := secret.DeepCopy()
					t.Data = data
					*secret = *t
				}
				return nil
			}),
		}
	}

	mockConfigMapClient := func(data map[string]string) *test.MockClient {
		return &test.MockClient{
			MockGet: test.NewMockGetFn(nil, func(obj runtime.Object) error {
				switch configMap := obj.(type) {
				case *v1.ConfigMap:
					t := configMap.DeepCopy()
					t.Data = data
					*configMap = *t
				}
				return nil
			}),
		}
	}

	testcases := map[string]struct {
		namespace                string
		name                     string
		workload                 *Workload
		client                   *test.MockClient
		hasError                 bool
		expectWorkloadSecretData []process.RequiredSecrets
		expectTraitSecretData    [][]process.RequiredSecrets
		expectConfigMapData      []map[string]string
	}{
		"workload is a secret consumer": {
			workload: &Workload{
				FullTemplate: &Template{
					TemplateStr: "output:" + secretConsumerTemplate,
				},
				Params: map[string]interface{}{
					"dbSecret": "test-workload",
				},
			},
			client: mockSecretClient(secretData),
			expectWorkloadSecretData: []process.RequiredSecrets{{
				Namespace:   "test-workload",
				Name:        "test-workload",
				ContextName: "dbConn",
				Data: map[string]interface{}{
					"username": "test-name",
					"password": "test-pwd",
				},
			}},
			namespace: "test-workload",
			name:      "test-workload",
		},
		"trait is a secret consumer": {
			workload: &Workload{
				FullTemplate: &Template{
					TemplateStr: `
output: parameter
parameter: {}
`,
				},
				Params: nil,
				Traits: []*Trait{{
					FullTemplate: &Template{
						TemplateStr: "outputs:" + secretConsumerTemplate,
					},
					Params: map[string]interface{}{
						"dbSecret": "test-trait",
					},
				}},
			},
			client: mockSecretClient(secretData),
			expectTraitSecretData: [][]process.RequiredSecrets{{{
				Namespace:   "test-trait",
				Name:        "test-trait",
				ContextName: "dbConn",
				Data: map[string]interface{}{
					"username": "test-name",
					"password": "test-pwd",
				},
			}}},
			namespace: "test-trait",
			name:      "test-trait",
		},
		"workload get config from configMap": {
			workload: &Workload{
				FullTemplate: &Template{
					TemplateStr: userConfigTemplate,
				},
				Params: map[string]interface{}{
					"image":  "busybox",
					"config": "test-config",
				},
			},
			client: mockConfigMapClient(map[string]string{"username": "test-configMap"}),
			expectConfigMapData: []map[string]string{{
				"name":  "username",
				"value": "test-configMap",
			}},
		},
	}

	for _, tc := range testcases {
		err := GetSecretAndConfigs(tc.client, tc.workload, tc.name, tc.namespace)
		assert.Equal(t, err != nil, tc.hasError)
		assert.DeepEqual(t, tc.expectWorkloadSecretData, tc.workload.RequiredSecrets)
		for i, tr := range tc.workload.Traits {
			assert.DeepEqual(t, tc.expectTraitSecretData[i], tr.RequiredSecrets)
		}
		assert.DeepEqual(t, tc.expectConfigMapData, tc.workload.UserConfigs)
	}
}
