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

package componentdefinition

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Apply ComponentDefinition to store its schema to ConfigMap Test", func() {
	ctx := context.Background()
	var ns corev1.Namespace

	Context("When the ComponentDefinition's namespace doesn't exist, should occur error", func() {
		It("Applying ComponentDefinition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: wd-without-ready-ns
  namespace: ns-def
  annotations:
    definition.oam.dev/description: "test"
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
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

			var def v1alpha2.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(validComponentDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Not(Succeed()))
		})
	})

	Context("When the ComponentDefinition without namespace is valid, should create a ConfigMap", func() {
		var componentDefinitionName = "web-no-ns"
		var namespace = "default"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: componentDefinitionName, Namespace: namespace}}

		It("Applying valid ComponentDefinition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: web-no-ns
  annotations:
    definition.oam.dev/description: "test"
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
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

			var def v1alpha2.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(validComponentDefinition), &def)).Should(BeNil())
			// API server will convert blank namespace to `default`
			def.Namespace = namespace
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())

			By("Check whether ConfigMap is created")
			reconcileRetry(&r, req)
			var cm corev1.ConfigMap
			name := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, componentDefinitionName)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)).Should(Succeed())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
		})
	})

	Context("When the ComponentDefinition with namespace is valid, should create a ConfigMap", func() {
		var componentDefinitionName = "web"
		var namespace = "ns-def"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: componentDefinitionName, Namespace: namespace}}

		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Applying valid ComponentDefinition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: web
  namespace: ns-def
  annotations:
    definition.oam.dev/description: "test"
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
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

			var def v1alpha2.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(validComponentDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())

			By("Check whether ConfigMap is created")
			reconcileRetry(&r, req)
			var cm corev1.ConfigMap
			name := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, componentDefinitionName)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)).Should(Succeed())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
		})
	})

	Context("When the ComponentDefinition is invalid, should hit issues", func() {
		var invalidComponentDefinitionName = "invalid-wd1"
		var namespace = "ns-def"
		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Applying invalid ComponentDefinition", func() {
			By("Apply the ComponentDefinition")
			var invalidComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: invalid-wd1
  namespace: ns-def
  annotations:
    definition.oam.dev/description: "test"
spec:
  definitionRef:
    name: deployments.apps
  schematic:
    cue:
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
        					image: "nginx:1.9.2"
        				}]
        			}
        		}
        	}
        }
`

			var invalidDef v1alpha2.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(invalidComponentDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			gotComponentDefinition := &v1alpha2.ComponentDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidComponentDefinitionName, Namespace: namespace}, gotComponentDefinition)).Should(BeNil())
		})
	})
})
