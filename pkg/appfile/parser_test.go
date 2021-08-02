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

	"github.com/crossplane/crossplane-runtime/pkg/test"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
	"github.com/oam-dev/kubevela/pkg/cue/process"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var expectedExceptApp = &Appfile{
	Name: "application-sample",
	Workloads: []*Workload{
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
	WorkflowSteps: []v1beta1.WorkflowStep{
		{
			Name: "suspend",
			Type: "suspend",
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
    - deployments.apps
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
      traits:
        - type: scaler
          properties:
            replicas: 10
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

var _ = Describe("Test application parser", func() {
	It("Test we can parse an application to an appFile", func() {
		o := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(appfileYaml), &o)
		Expect(err).ShouldNot(HaveOccurred())

		// Create mock client
		tclient := test.MockClient{
			MockGet: func(ctx context.Context, key client.ObjectKey, obj client.Object) error {
				if strings.Contains(key.Name, "notexist") {
					return &errors2.StatusError{ErrStatus: metav1.Status{Reason: "NotFound", Message: "not found"}}
				}
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

		appfile, err := NewApplicationParser(&tclient, dm, pd).GenerateAppFile(context.TODO(), &o)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(equal(expectedExceptApp, appfile)).Should(BeTrue())

		notfound := v1beta1.Application{}
		err = yaml.Unmarshal([]byte(appfileYaml2), &notfound)
		Expect(err).ShouldNot(HaveOccurred())
		_, err = NewApplicationParser(&tclient, dm, pd).GenerateAppFile(context.TODO(), &notfound)
		Expect(err).Should(HaveOccurred())
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
			Namespace:    "default",
			Workloads: []*Workload{
				{
					Name: "myweb",
					Type: "worker",
					Params: map[string]interface{}{
						"image":  "busybox",
						"cmd":    []interface{}{"sleep", "1000"},
						"config": "myconfig",
					},
					UserConfigs: []map[string]string{
						{"name": "c1", "value": "v1"},
						{"name": "c2", "value": "v2"},
					},
					Scopes: []Scope{
						{Name: "test-scope", GVK: schema.GroupVersionKind{
							Group:   "core.oam.dev",
							Version: "v1alpha2",
							Kind:    "HealthScope",
						}},
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
					},
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
		comps, err := TestApp.GenerateComponentManifests()
		Expect(err).To(BeNil())

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

		expectCompManifest := &types.ComponentManifest{
			Name:             "myweb",
			StandardWorkload: expectWorkload,
			Traits: []*unstructured.Unstructured{
				{
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
				},
			},
			Scopes: []*corev1.ObjectReference{
				{
					APIVersion: "core.oam.dev/v1alpha2",
					Kind:       "HealthScope",
					Name:       "test-scope",
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
		Expect(len(comps)).To(BeEquivalentTo(1))
		comp := comps[0]
		Expect(comp.Name).Should(Equal(expectCompManifest.Name))
		Expect(comp.Traits).Should(Equal(expectCompManifest.Traits))
		Expect(comp.Scopes).Should(Equal(expectCompManifest.Scopes))
		Expect(comp.StandardWorkload).Should(SatisfyAny(
			BeEquivalentTo(expectWorkload),
			BeEquivalentTo(expectWorkloadOptional)))
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
				Name:         "abc",
				FullTemplate: &Template{TemplateStr: template},
			}
			By("call target function")
			secrets, err := parseInsertSecretTo(ctx, k8sClient, ns, wl.FullTemplate.TemplateStr, wl.Params)
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
				FullTemplate: &Template{TemplateStr: template},
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
			secrets, err := parseInsertSecretTo(ctx, k8sClient, ns, wl.FullTemplate.TemplateStr, wl.Params)
			Expect(err).Should(BeNil())
			Expect(secrets).Should(Equal(targetRequiredSecret))
		})
	})
})

var _ = Describe("Test IsSecretProducer", func() {
	Context("Workload is a Cloud Resource producer", func() {
		It("", func() {
			var targetSecretName = "db-conn"
			wl := &Workload{
				Params: map[string]interface{}{
					"outputSecretName": targetSecretName,
				},
			}
			Expect(wl.IsSecretProducer()).Should(Equal(true))
		})
	})

	Context("Workload is a Cloud Resource producer", func() {
		It("", func() {
			wl := &Workload{}
			Expect(wl.IsSecretProducer()).Should(Equal(false))
		})
	})
})

var _ = Describe("Test IsSecretConsumer", func() {
	Context("Workload is a Cloud Resource consumer", func() {
		It("", func() {
			wl := &Workload{
				FullTemplate: &Template{TemplateStr: "// +insertSecretTo=dbConn"},
			}
			Expect(wl.IsSecretConsumer()).Should(Equal(true))
		})
	})

	Context("Workload is a Cloud Resource consumer", func() {
		It("", func() {
			wl := &Workload{
				FullTemplate: &Template{TemplateStr: "// +useage=dbConn"},
			}
			Expect(wl.IsSecretProducer()).Should(Equal(false))
		})
	})
})
