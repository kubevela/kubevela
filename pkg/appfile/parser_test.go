/*
Copyright 2020 The KubeVela Authors.

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

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/ghodss/yaml"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	oamtypes "github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/dsl/definition"
	"github.com/oam-dev/kubevela/pkg/dsl/process"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var expectedExceptApp = &Appfile{
	Name: "test",
	Workloads: []*Workload{
		{
			Name: "myweb",
			Type: "worker",
			Params: map[string]interface{}{
				"image": "busybox",
				"cmd":   []interface{}{"sleep", "1000"},
			},
			Template: `
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
			Traits: []*Trait{
				{
					Name: "scaler",
					Params: map[string]interface{}{
						"replicas": float64(10),
					},
					Template: `
      outputs:scaler: {
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
		},
	},
}

const traitDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Manually scale the app"
  name: scaler
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  extension:
    template: |-
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
      }`

const componenetDefinition = `
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

const appfileYaml = `
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: application-sample
spec:
  components:
    - name: myweb
      type: worker
      properties:
        image: "busybox"
        cmd:
        - sleep
        - "1000"
      traits:
        - type: scaler
          properties:
            replicas: 10
`

var _ = Describe("Test application parser", func() {
	It("Test we can parse an application to an appFile", func() {
		o := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(appfileYaml), &o)
		Expect(err).ShouldNot(HaveOccurred())

		// Create mock client
		tclient := test.MockClient{
			MockGet: func(ctx context.Context, key types.NamespacedName, obj runtime.Object) error {
				switch o := obj.(type) {
				case *v1beta1.ComponentDefinition:
					wd, err := util.UnMarshalStringToComponentDefinition(componenetDefinition)
					if err != nil {
						return err
					}
					*o = *wd
				case *v1beta1.TraitDefinition:
					td, err := util.UnMarshalStringToTraitDefinition(traitDefinition)
					if err != nil {
						return err
					}
					*o = *td
				}
				return nil
			},
		}

		appfile, err := NewApplicationParser(&tclient, dm, pd).GenerateAppFile(context.TODO(), "test", &o)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(equal(expectedExceptApp, appfile)).Should(BeTrue())
	})
})

func equal(af, dest *Appfile) bool {
	if af.Name != dest.Name || len(af.Workloads) != len(dest.Workloads) {
		return false
	}
	for i, wd := range af.Workloads {
		destWd := dest.Workloads[i]
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

var _ = Describe("Test appFile parser", func() {
	It("application without-trait will only create appfile with workload", func() {
		// TestApp is test data
		var TestApp = &Appfile{
			RevisionName: "test-v1",
			Name:         "test",
			Workloads: []*Workload{
				{
					Name: "myweb",
					Type: "worker",
					Params: map[string]interface{}{
						"image":  "busybox",
						"cmd":    []interface{}{"sleep", "1000"},
						"config": "myconfig",
					},
					Scopes: []Scope{
						{Name: "test-scope", GVK: schema.GroupVersionKind{
							Group:   "core.oam.dev",
							Version: "v1alpha2",
							Kind:    "HealthScope",
						}},
					},
					engine: definition.NewWorkloadAbstractEngine("myweb", pd),
					Template: `
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
      					if context["config"] != _|_ {
      						env: context.config
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
				},
			},
		}
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "kubevela-test-myweb-myconfig", Namespace: "default"},
			Data:       map[string]string{"c1": "v1", "c2": "v2"},
		}
		Expect(k8sClient.Create(context.Background(), cm.DeepCopy())).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		ac, components, err := NewApplicationParser(k8sClient, dm, pd).GenerateApplicationConfiguration(TestApp, "default")
		Expect(err).To(BeNil())
		manuscaler := util.Object2RawExtension(&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "core.oam.dev/v1alpha2",
				"kind":       "ManualScalerTrait",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"app.oam.dev/component":   "myweb",
						"app.oam.dev/appRevision": "test-v1",
						"app.oam.dev/name":        "test",
						"trait.oam.dev/type":      "scaler",
						"trait.oam.dev/resource":  "scaler",
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
				Name:      "test",
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: "test"},
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{
						ComponentName: "myweb",
						Scopes: []v1alpha2.ComponentScope{
							{
								ScopeReference: v1alpha1.TypedReference{
									APIVersion: "core.oam.dev/v1alpha2",
									Kind:       "HealthScope",
									Name:       "test-scope",
								},
							},
						},
						Traits: []v1alpha2.ComponentTrait{
							{
								Trait: manuscaler,
							},
						},
					},
				},
			},
		}
		fmt.Println(cmp.Diff(expectAppConfig, ac))
		Expect(assert.ObjectsAreEqual(expectAppConfig, ac)).To(Equal(true))

		expectComponent := &v1alpha2.Component{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Component",
				APIVersion: "core.oam.dev/v1alpha2",
			}, ObjectMeta: metav1.ObjectMeta{
				Name:      "myweb",
				Namespace: "default",
				Labels:    map[string]string{oam.LabelAppName: "test"},
			}}
		expectWorkload := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						"workload.oam.dev/type":   "worker",
						"app.oam.dev/component":   "myweb",
						"app.oam.dev/appRevision": "test-v1",
						"app.oam.dev/name":        "test",
					},
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"app.oam.dev/component": "myweb"}},
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{"labels": map[string]interface{}{"app.oam.dev/component": "myweb"}},
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"command": []interface{}{"sleep", "1000"},
									"image":   "busybox",
									"name":    "myweb",
									"env": []interface{}{
										map[string]interface{}{"name": "c1", "value": "v1"},
										map[string]interface{}{"name": "c2", "value": "v2"},
									},
								},
							},
						},
					},
				},
			},
		}
		// assertion util cannot compare slices embedded in map correctly while slice order is not required
		// e.g., .containers[0].env in this case
		// as a workaround, prepare two expected targets covering all possible slice order
		// if any one is satisfied, the equal assertion pass
		expectWorkloadOptional := expectWorkload.DeepCopy()
		unstructured.SetNestedSlice(expectWorkloadOptional.Object, []interface{}{
			map[string]interface{}{
				"command": []interface{}{"sleep", "1000"},
				"image":   "busybox",
				"name":    "myweb",
				"env": []interface{}{
					map[string]interface{}{"name": "c2", "value": "v2"},
					map[string]interface{}{"name": "c1", "value": "v1"},
				},
			},
		}, "spec", "template", "spec", "containers")

		By(" built components' length must be 1")
		Expect(len(components)).To(BeEquivalentTo(1))
		Expect(components[0].ObjectMeta).To(BeEquivalentTo(expectComponent.ObjectMeta))
		Expect(components[0].TypeMeta).To(BeEquivalentTo(expectComponent.TypeMeta))
		Expect(components[0].Spec.Workload).Should(SatisfyAny(
			BeEquivalentTo(util.Object2RawExtension(expectWorkload)),
			BeEquivalentTo(util.Object2RawExtension(expectWorkloadOptional))))
	})

})

var _ = Describe("Test appfile parser to parse helm module", func() {
	var (
		appName  = "test-app"
		compName = "test-comp"
	)

	It("Test application containing helm module", func() {
		appFile := &Appfile{
			Name:         appName,
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
					DefinitionReference: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
				},
			},
		}
		By("Generate ApplicationConfiguration and Components")
		ac, components, err := NewApplicationParser(k8sClient, dm, pd).GenerateApplicationConfiguration(appFile, "default")
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

var _ = Describe("Test appfile parser to parse kube module", func() {
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
					FullTemplate: &util.Template{
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
					},
					DefinitionReference: common.WorkloadGVK{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
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

	It("Test application containing kube module", func() {
		By("Generate ApplicationConfiguration and Components")
		ac, components, err := NewApplicationParser(k8sClient, dm, pd).GenerateApplicationConfiguration(testAppfile(), "default")
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
		_, _, err := NewApplicationParser(k8sClient, dm, pd).GenerateApplicationConfiguration(appfile, "default")

		expectError := errors.WithMessage(errors.New(`require parameter "image"`), "cannot resolve parameter settings")
		diff := cmp.Diff(expectError, err, test.EquateErrors())
		Expect(diff).Should(BeEmpty())
	})
})

var _ = Describe("Test Get OutputSecretNames", func() {
	Context("Workload will generate cloud resource secret", func() {
		It("", func() {
			var targetSecretName = "db-conn"
			wl := &Workload{
				Params: map[string]interface{}{
					"outputSecretName": targetSecretName,
				},
			}
			name, err := GetOutputSecretNames(wl)
			Expect(err).Should(BeNil())
			Expect(name).Should(Equal(targetSecretName))
		})
	})

	Context("Workload will not generate cloud resource secret", func() {
		It("", func() {
			wl := &Workload{}
			name, err := GetOutputSecretNames(wl)
			Expect(err).ShouldNot(BeNil())
			Expect(name).Should(Equal(""))
		})
	})
})

var _ = Describe("Test parsing Workload's insertSecretTo tag", func() {
	var (
		ctx              = context.Background()
		ns               = "default"
		targetSecretName = "db-conn"
		data             = map[string][]byte{
			"endpoint": []byte("aaa"),
			"password": []byte("bbb"),
			"username": []byte("ccc"),
		}
	)

	Context("Workload template is not valid", func() {
		It("", func() {
			var (
				template = `
settings: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string

	// +usage=Commands to run in the container
	cmd?: [...string]

	// +usage=Which port do you want customer traffic sent to
	// +short=p
	port: *80 | int

	// +usage=Referred db secret
	// +insertSecretTo=dbConn
	dbSecret?: string

	// +usage=Number of CPU units for the service
	cpu?: string
}
`
			)

			wl := &Workload{
				Name:     "abc",
				Template: template,
			}
			By("call target function")
			secrets, err := parseWorkloadInsertSecretTo(ctx, k8sClient, ns, wl)
			Expect(err).Should(BeNil())
			Expect(secrets).Should(BeNil())
		})
	})

	Context("Workload will generate cloud resource secret", func() {
		It("", func() {
			var (
				template = `
parameter: {
	// +usage=Which image would you like to use for your service
	// +short=i
	image: string

	// +usage=Commands to run in the container
	cmd?: [...string]

	// +usage=Which port do you want customer traffic sent to
	// +short=p
	port: *80 | int

	// +usage=Referred db secret
	// +insertSecretTo=dbConn
	dbSecret?: string

	// +usage=Number of CPU units for the service
	cpu?: string
}
`
			)

			wl := &Workload{
				Name: "abc",
				Params: map[string]interface{}{
					"dbSecret": targetSecretName,
				},
				Template: template,
			}
			By("create secret")
			s := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "db-conn",
					Namespace: "default",
				},
				Data: data,
			}
			targetRequiredSecret := []process.RequiredSecrets{
				{
					Name:        targetSecretName,
					ContextName: "dbConn",
					Namespace:   ns,
					Data: map[string]interface{}{
						"endpoint": "aaa",
						"password": "bbb",
						"username": "ccc",
					},
				},
			}
			err := k8sClient.Create(ctx, s)
			Expect(err).Should(BeNil())
			By("call target function")
			secrets, err := parseWorkloadInsertSecretTo(ctx, k8sClient, ns, wl)
			Expect(err).Should(BeNil())
			Expect(secrets).Should(Equal(targetRequiredSecret))
		})
	})
})

var _ = Describe("Test IsCloudResourceProducer", func() {
	Context("Workload is a Cloud Resource producer", func() {
		It("", func() {
			var targetSecretName = "db-conn"
			wl := &Workload{
				Params: map[string]interface{}{
					"outputSecretName": targetSecretName,
				},
			}
			Expect(wl.IsCloudResourceProducer()).Should(Equal(true))
		})
	})

	Context("Workload is a Cloud Resource producer", func() {
		It("", func() {
			wl := &Workload{}
			Expect(wl.IsCloudResourceProducer()).Should(Equal(false))
		})
	})
})

var _ = Describe("Test IsCloudResourceConsumer", func() {
	Context("Workload is a Cloud Resource consumer", func() {
		It("", func() {
			wl := &Workload{
				Template: "// +insertSecretTo=dbConn",
			}
			Expect(wl.IsCloudResourceConsumer()).Should(Equal(true))
		})
	})

	Context("Workload is a Cloud Resource consumer", func() {
		It("", func() {
			wl := &Workload{
				Template: "// +useage=dbConn",
			}
			Expect(wl.IsCloudResourceProducer()).Should(Equal(false))
		})
	})
})
