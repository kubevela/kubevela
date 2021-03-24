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
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Test ComponentDefinition Controller", func() {
	ctx := context.Background()
	var ns corev1.Namespace

	Context("When the ComponentDefinition's namespace doesn't exist, should occur error", func() {
		It("Applying ComponentDefinition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: cd-without-ready-ns
  namespace: ns-def
  annotations:
    definition.oam.dev/description: "test"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 10*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 10*time.Second, time.Second).Should(Equal(name))
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
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 10*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 10*time.Second, time.Second).Should(Equal(name))
		})
	})

	Context("When the ComponentDefinition is invalid, should hit issues", func() {
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

		It("Applying a ComponentDefinition without paramter", func() {
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
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
			var invalidComponentDefinitionName = "invalid-wd1"
			Expect(yaml.Unmarshal([]byte(invalidComponentDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: invalidComponentDefinitionName, Namespace: namespace}}
			reconcileRetry(&r, req)
			gotComponentDefinition := &v1alpha2.ComponentDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidComponentDefinitionName, Namespace: namespace}, gotComponentDefinition)).Should(BeNil())
		})

		It("Applying a ComponentDefinition with an invalid Workload.Definition", func() {
			var invalidComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: invalid-wd2
  namespace: ns-def
  annotations:
    definition.oam.dev/description: "test"
spec:
  workload:
    definition:
      apiVersion: /apps/v1/
      kind: Deployment
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
			var invalidDef v1alpha2.ComponentDefinition
			var invalidComponentDefinitionName = "invalid-wd2"
			Expect(yaml.Unmarshal([]byte(invalidComponentDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			By("Check whether WorkloadDefinition is created")
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: invalidComponentDefinitionName, Namespace: namespace}}
			reconcileRetry(&r, req)
			var wd v1alpha2.WorkloadDefinition
			var wdName = invalidComponentDefinitionName
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: wdName}, &wd)).Should(Not(Succeed()))
		})
	})

	Context("When the ComponentDefinition only contains Workload.Definition, should create a WorkloadDefinition", func() {
		var componentDefinitionName = "cd-with-workload-definition"
		var namespace = "default"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: componentDefinitionName, Namespace: namespace}}

		It("Applying ComponentDefinition with Workload.Definition", func() {
			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: cd-with-workload-definition
  annotations:
    definition.oam.dev/description: "test"
spec:
  workload:
    definition:
      apiVersion: apps/v1
      kind: Deployment
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
			def.Namespace = namespace
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())

			By("Check whether WorkloadDefinition is created")
			reconcileRetry(&r, req)
			var wd v1alpha2.WorkloadDefinition
			var wdName = componentDefinitionName
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: wdName}, &wd)
				return err == nil
			}, 10*time.Second, time.Second).Should(BeTrue())
			Expect(wd.Name).Should(Equal(def.Name))
			Expect(wd.Namespace).Should(Equal(def.Namespace))
			Expect(wd.Annotations).Should(Equal(def.Annotations))
			Expect(wd.Spec.Schematic).Should(Equal(def.Spec.Schematic))
		})
	})

	Context("When the ComponentDefinition contains Helm schematic", func() {
		var componentDefinitionName = "cd-with-helm-schematic"
		var namespace = "default"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: componentDefinitionName, Namespace: namespace}}

		It("Applying ComponentDefinition with Helm schematic", func() {
			cd := v1alpha2.ComponentDefinition{}
			cd.SetName(componentDefinitionName)
			cd.SetNamespace(namespace)
			cd.Spec.Workload.Definition = common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}
			cd.Spec.Schematic = &common.Schematic{
				HELM: &common.Helm{
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
			}
			By("Create ComponentDefinition")
			Expect(k8sClient.Create(ctx, &cd)).Should(Succeed())

			By("Check whether WorkloadDefinition is created")
			reconcileRetry(&r, req)
			var wd v1alpha2.WorkloadDefinition
			var wdName = componentDefinitionName
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: wdName}, &wd)
				return err == nil
			}, 10*time.Second, time.Second).Should(BeTrue())
			Expect(wd.Name).Should(Equal(cd.Name))
			Expect(wd.Namespace).Should(Equal(cd.Namespace))
			Expect(wd.Annotations).Should(Equal(cd.Annotations))
			Expect(wd.Spec.Schematic).Should(Equal(cd.Spec.Schematic))
		})
	})

	Context("When the ComponentDefinition contain Workload.Type, shouldn't create a WorkloadDefinition", func() {
		var componentDefinitionName = "cd-with-workload-type"
		var namespace = "default"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: componentDefinitionName, Namespace: namespace}}

		It("Applying ComponentDefinition with Workload.Type", func() {
			By("Apply WorkloadDefinition")
			var taskWorkloadDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: WorkloadDefinition
metadata:
  name: worker
  annotations:
    definition.oam.dev/description: "Describes long-running, scalable, containerized services that running at backend. They do NOT have network endpoint to receive external network traffic."
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
			var task v1alpha2.WorkloadDefinition
			Expect(yaml.Unmarshal([]byte(taskWorkloadDefinition), &task)).Should(BeNil())
			task.Namespace = namespace
			Expect(k8sClient.Create(ctx, &task)).Should(Succeed())

			By("Apply ComponentDefinition")
			var validComponentDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: ComponentDefinition
metadata:
  name: cd-with-workload-type
spec:
  workload:
    type: worker
`
			var def v1alpha2.ComponentDefinition
			Expect(yaml.Unmarshal([]byte(validComponentDefinition), &def)).Should(BeNil())
			def.Namespace = namespace
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())

			By("Check whether WorkloadDefinition is created")
			reconcileRetry(&r, req)
			var wd v1alpha2.WorkloadDefinition
			var wdName = componentDefinitionName
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: wdName}, &wd)).Should(Not(Succeed()))

			By("Check whether ConfigMap is created")
			var cm corev1.ConfigMap
			name := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, componentDefinitionName)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 10*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 10*time.Second, time.Second).Should(Equal(name))
		})
	})

})
