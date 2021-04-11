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
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/oam"
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
						Reference: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
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
		ac, components, err := appFile.GenerateApplicationConfiguration()
		Expect(err).To(BeNil())

		manuscaler := util.Object2RawExtension(&unstructured.Unstructured{
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
		})
		expectAppConfig := &v1alpha2.ApplicationConfiguration{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ApplicationConfiguration",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: appName},
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: compName,
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: manuscaler,
							},
						},
					},
				},
			},
		}
		expectComponent := &v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      compName,
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: appName},
			},
			Spec: v1alpha2.ComponentSpec{
				Helm: &common.Helm{
					Release: util.Object2RawExtension(map[string]interface{}{
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
					}),
					Repository: util.Object2RawExtension(map[string]interface{}{
						"apiVersion": "source.toolkit.fluxcd.io/v1beta1",
						"kind":       "HelmRepository",
						"metadata": map[string]interface{}{
							"name":      fmt.Sprintf("%s-%s", appName, compName),
							"namespace": "default",
						},
						"spec": map[string]interface{}{
							"url": "http://oam.dev/catalog/",
						},
					}),
				},
				Workload: util.Object2RawExtension(map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"labels": map[string]interface{}{
							"workload.oam.dev/type":   "webapp-chart",
							"app.oam.dev/component":   compName,
							"app.oam.dev/name":        appName,
							"app.oam.dev/appRevision": appName + "-v1",
						},
					},
				}),
			},
		}
		By("Verify expected ApplicationConfiguration")
		diff := cmp.Diff(ac, expectAppConfig)
		Expect(diff).Should(BeEmpty())
		By("Verify expected Component")
		diff = cmp.Diff(components[0], expectComponent)
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
	var expectWorkload = func() runtime.RawExtension {
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
						Reference: common.WorkloadGVK{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
						},
					},
				},
			},
		}

	}

	manuscaler := util.Object2RawExtension(&unstructured.Unstructured{
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
	})

	It("Test generate AppConfig resources from Kube schematic", func() {
		By("Generate ApplicationConfiguration and Components")
		ac, components, err := testAppfile().GenerateApplicationConfiguration()
		Expect(err).To(BeNil())

		expectAppConfig := &v1alpha2.ApplicationConfiguration{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ApplicationConfiguration",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: appName},
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: compName,
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: manuscaler,
							},
						},
					},
				},
			},
		}
		expectComponent := &v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      compName,
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: appName},
			},
			Spec: v1alpha2.ComponentSpec{
				Workload: expectWorkload(),
			},
		}
		By("Verify expected ApplicationConfiguration")
		diff := cmp.Diff(ac, expectAppConfig)
		Expect(diff).Should(BeEmpty())
		By("Verify expected Component")
		diff = cmp.Diff(components[0], expectComponent)
		Expect(diff).ShouldNot(BeEmpty())
	})

	It("Test missing set required parameter", func() {
		appfile := testAppfile()
		// remove parameter settings
		appfile.Workloads[0].Params = nil
		_, _, err := appfile.GenerateApplicationConfiguration()

		expectError := errors.WithMessage(errors.New(`require parameter "image"`), "cannot resolve parameter settings")
		diff := cmp.Diff(expectError, err, test.EquateErrors())
		Expect(diff).Should(BeEmpty())
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
