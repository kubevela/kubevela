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

	"github.com/google/go-cmp/cmp"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/cue/definition"
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
		},
	},
	WorkflowSteps: []v1beta1.WorkflowStep{
		{
			Name: "suspend",
			Type: "suspend",
		},
	},
}

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
			AppRevisionName: "test-v1",
			Name:            "test",
			Namespace:       "default",
			RelatedTraitDefinitions: map[string]*v1beta1.TraitDefinition{
				"scaler": {
					Spec: v1beta1.TraitDefinitionSpec{},
				},
			},
			Workloads: []*Workload{
				{
					Name: "myweb",
					Type: "worker",
					Params: map[string]interface{}{
						"image": "busybox",
						"cmd":   []interface{}{"sleep", "1000"},
					},
					Scopes: []Scope{
						{Name: "test-scope", GVK: metav1.GroupVersionKind{
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
					"name":      "myweb",
					"namespace": "default",
					"labels": map[string]interface{}{
						"workload.oam.dev/type":    "worker",
						"app.oam.dev/component":    "myweb",
						"app.oam.dev/appRevision":  "test-v1",
						"app.oam.dev/name":         "test",
						"app.oam.dev/resourceType": "WORKLOAD",
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
			},
		}, "spec", "template", "spec", "containers")

		By(" built components' length must be 1")
		Expect(len(comps)).To(BeEquivalentTo(1))
		comp := comps[0]
		Expect(comp.Name).Should(Equal(expectCompManifest.Name))
		Expect(cmp.Diff(comp.Traits, expectCompManifest.Traits)).Should(BeEmpty())
		Expect(comp.Scopes).Should(Equal(expectCompManifest.Scopes))
		Expect(cmp.Diff(comp.StandardWorkload, expectWorkloadOptional)).Should(BeEmpty())
	})

})
