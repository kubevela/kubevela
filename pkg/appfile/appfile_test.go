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
	"fmt"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/google/go-cmp/cmp"
	terraformtypes "github.com/oam-dev/terraform-controller/api/types/crossplane-runtime"
	terraformapi "github.com/oam-dev/terraform-controller/api/v1beta2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
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

var _ = Describe("Test Helm schematic appfile", func() {
	var (
		appName      = "test-app"
		compName     = "test-comp"
		workloadName = "test-workload"
	)

	It("Test generate AppConfig resources from Helm schematic", func() {
		appFile := &Appfile{
			Name:            appName,
			Namespace:       "default",
			AppRevisionName: appName + "-v1",
			RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
				"scaler": {
					Spec: v1beta1.TraitDefinitionSpec{},
				},
			},
			Workloads: []*Workload{
				{
					Name:               workloadName,
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
							Release: *util.Object2RawExtension(map[string]interface{}{
								"chart": map[string]interface{}{
									"spec": map[string]interface{}{
										"chart":   "podinfo",
										"version": "5.1.4",
									},
								},
							}),
							Repository: *util.Object2RawExtension(map[string]interface{}{
								"url": "https://charts.kubevela.net/example/",
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
			Name: workloadName,
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
							"url": "https://charts.kubevela.net/example/",
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
			AppRevisionName: appName + "-v1",
			Name:            appName,
			Namespace:       "default",
			RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
				"scaler": {
					Spec: v1beta1.TraitDefinitionSpec{},
				},
			},
			Workloads: []*Workload{
				{
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
						},
					},
					FullTemplate: &Template{
						Kube: &common.Kube{
							Template: testTemplate(),
							Parameters: []common.KubeParameter{
								{
									Name:       "image",
									ValueType:  common.StringType,
									Required:   pointer.Bool(true),
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

var _ = Describe("Test Policy", func() {
	It("test generate PolicyWorkloads", func() {
		testAppfile := &Appfile{
			Name:      "test-app",
			Namespace: "default",
			Workloads: []*Workload{
				{
					Name:               "test-comp",
					Type:               "worker",
					CapabilityCategory: oamtypes.KubeCategory,
					engine:             definition.NewWorkloadAbstractEngine("test-comp", pd),
					FullTemplate: &Template{
						Kube: &common.Kube{
							Template: func() runtime.RawExtension {
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
							}(),
						},
					},
				},
			},
			PolicyWorkloads: []*Workload{
				{
					Name: "test-policy",
					Type: "test-policy",
					Params: map[string]interface{}{
						"boundComponents": []string{"test-comp"},
					},
					FullTemplate: &Template{TemplateStr: `	output: {
		apiVersion: "core.oam.dev/v1alpha2"
		kind:       "HealthScope"
		spec: {
					for k, v in parameter.boundComponents {
						compName: v
						workload: {
							apiVersion: context.artifacts[v].workload.apiVersion
							kind:       context.artifacts[v].workload.kind
							name:       v
						}
					},
		}
	}
	outputs: virtualservice: {
		apiVersion: "networking.istio.io/v1alpha3"
		kind:       "VirtualService"
		spec: {
			hosts: "abc"
			http: ["abc"]
		}
	}
	parameter: {
		boundComponents: [...string]
	}`},
					engine: definition.NewWorkloadAbstractEngine("test-policy", pd),
				},
			},
			app: &v1beta1.Application{},
		}
		_, err := testAppfile.GenerateComponentManifests()
		Expect(err).Should(BeNil())
		testAppfile.parser = &Parser{client: k8sClient}
		gotPolicies, err := testAppfile.GeneratePolicyManifests(context.Background())
		Expect(err).Should(BeNil())
		Expect(len(gotPolicies)).ShouldNot(Equal(0))

		expectPolicy0 := unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"compName": "test-comp",
					"workload": map[string]interface{}{
						"name":       "test-comp",
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
					},
				},
				"metadata": map[string]interface{}{
					"name":      "test-policy",
					"namespace": "default",
					"labels": map[string]interface{}{
						"app.oam.dev/name":        "test-app",
						"app.oam.dev/component":   "test-policy",
						"app.oam.dev/appRevision": "",
					},
				},
				"apiVersion": "core.oam.dev/v1alpha2",
				"kind":       "HealthScope",
			},
		}
		Expect(len(gotPolicies)).Should(Equal(2))
		gotPolicy := gotPolicies[0]
		Expect(cmp.Diff(gotPolicy.Object, expectPolicy0.Object)).Should(BeEmpty())
		expectPolicy1 := unstructured.Unstructured{
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"hosts": "abc",
					"http":  []interface{}{"abc"},
				},
				"metadata": map[string]interface{}{
					"name":      "test-policy",
					"namespace": "default",
					"labels": map[string]interface{}{
						"app.oam.dev/name":        "test-app",
						"app.oam.dev/component":   "test-policy",
						"app.oam.dev/appRevision": "",
					},
				},
				"apiVersion": "networking.istio.io/v1alpha3",
				"kind":       "VirtualService",
			},
		}
		gotPolicy = gotPolicies[1]
		Expect(cmp.Diff(gotPolicy.Object, expectPolicy1.Object)).Should(BeEmpty())
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
			Workloads:       []*Workload{wl},
			Name:            appName,
			AppRevisionName: revision,
			Namespace:       ns,
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
		Required:   pointer.Bool(true),
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
							Required:   pointer.Bool(true),
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
			metadata: {
				labels: {
					app: "nginx"
				}
			}
			spec: {
				containers: [{
					image: "nginx:1.14.0"
					name:  "nginx"
				}]
				ports: [{
					containerPort: 80
				}]
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
							Required:   pointer.Bool(true),
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
							Required:   pointer.Bool(true),
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

	for i, tc := range testcases {
		template, err := GenerateCUETemplate(tc.workload)
		assert.Equal(t, err != nil, tc.hasError)
		if tc.hasError {
			assert.Equal(t, tc.errInfo, err.Error())
			continue
		}
		assert.Equal(t, tc.expectData, template, i)
	}
}

func TestPrepareArtifactsData(t *testing.T) {
	compManifests := []*oamtypes.ComponentManifest{
		{
			Name:         "readyComp",
			Namespace:    "ns",
			RevisionName: "readyComp-v1",
			StandardWorkload: &unstructured.Unstructured{Object: map[string]interface{}{
				"fake": "workload",
			}},
			Traits: func() []*unstructured.Unstructured {
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
	assert.NilError(t, err)
	diff := cmp.Diff(gotWorkload, map[string]interface{}{"fake": string("workload")})
	assert.Equal(t, diff, "")

	_, gotIngress, err := unstructured.NestedMap(gotArtifacts, "readyComp", "traits", "ingress", "ingress")
	assert.NilError(t, err)
	if !gotIngress {
		t.Fatalf("cannot get ingress trait")
	}
	_, gotSvc, err := unstructured.NestedMap(gotArtifacts, "readyComp", "traits", "ingress", "service")
	assert.NilError(t, err)
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
	assert.NilError(t, err)
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
	wl := &Workload{Type: "stateful", Traits: []*Trait{tr}}
	cm, err := baseGenerateComponent(pContext, wl, appName, ns)
	assert.NilError(t, err)
	assert.Equal(t, cm.Traits[0].Object["kind"], "StatefulSet")
	assert.Equal(t, cm.Traits[0].Object["workflowName"], workflowName)
	assert.Equal(t, cm.Traits[0].Object["publishVersion"], publishVersion)
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
			Workloads: []*Workload{
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
		runtime.DefaultUnstructuredConverter.FromUnstructured(componentManifests[0].StandardWorkload.Object, deployment)
		labels := deployment.Spec.Template.Labels
		annotations := deployment.Spec.Template.Annotations
		Expect(cmp.Diff(len(labels), 2)).Should(BeEmpty())
		Expect(cmp.Diff(len(annotations), 2)).Should(BeEmpty())
		Expect(cmp.Diff(labels["lk1"], "lv1")).Should(BeEmpty())
		Expect(cmp.Diff(annotations["ak1"], "av1")).Should(BeEmpty())
	})

})
