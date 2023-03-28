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

package utils

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test Capability", func() {
	ctx := context.Background()
	var (
		namespace = "ns-cap"
		ns        corev1.Namespace
	)

	BeforeEach(func() {
		ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
		By("Create a namespace")
		Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
	})

	Context("When the definition is ComponentDefinition", func() {
		var componentDefinitionName = "web1"

		It("Test CapabilityComponentDefinition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: web1
  namespace: ns-cap
  annotations:
    definition.oam.dev/description: "test"
spec:
  workload:
    type: deployments.apps
  schematic:
    cue:
      template: |
        outputs: {
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
        	}
        }
        parameter: {
        	// +usage=Which image would you like to use for your service
        	// +short=i
        	image: string

        	// +usage=Commands to run in the container
        	cmd?: [...string]
        }

`
			var componentDefinition v1beta1.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(validComponentDefinition), &componentDefinition)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &componentDefinition)).Should(Succeed())

			By("Test GetCapabilityObject")
			def := &CapabilityComponentDefinition{Name: componentDefinitionName, ComponentDefinition: *componentDefinition.DeepCopy()}

			By("Test GetOpenAPISchema")
			schema, err := def.GetOpenAPISchema(namespace)
			Expect(err).Should(BeNil())
			Expect(schema).Should(Not(BeNil()))
		})
	})

	Context("When the definition is CapabilityBaseDefinition", func() {

		It("Test CapabilityTraitDefinition", func() {
			By("Test CreateOrUpdateConfigMap")
			definitionName := "n1"
			def := &CapabilityBaseDefinition{}
			ownerReference := []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "k1",
				Name:               definitionName,
				UID:                "123456",
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}}
			_, err := def.CreateOrUpdateConfigMap(ctx, k8sClient, namespace, definitionName, typeTraitDefinition, nil, nil, []byte(""), ownerReference)
			Expect(err).Should(BeNil())
		})
	})
})
