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

package policydefinition

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

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Apply PolicyDefinition to store its schema to ConfigMap Test", func() {
	ctx := context.Background()
	var ns corev1.Namespace

	Context("When the PolicyDefinition is valid, but the namespace doesn't exist, should occur errors", func() {
		It("Apply PolicyDefinition", func() {
			By("Apply PolicyDefinition")
			var validPolicyDefinition = `
apiVersion: core.oam.dev/v1beta1
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your policy
  name: apply-object
  namespace: not-exist
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "core.oam.dev/v1alpha1"
          kind:       "EnvBinding"
          spec: {
            engine: parameter.engine
            appTemplate: {
              apiVersion: "core.oam.dev/v1beta1"
              kind:       "Application"
              metadata: {
                name:      context.appName
                namespace: context.namespace
              }
              spec: {
                components: context.components
              }
            }
            envs: parameter.envs
          }
        }

        #Env: {
          name: string
          patch: components: [...{
            name: string
            type: string
            properties: {...}
          }]
          placement: clusterSelector: {
            labels?: [string]: string
            name?: string
          }
        }

        parameter: {
          engine: *"ocm" | string
          envs: [...#Env]
        }
`

			var def v1beta1.PolicyDefinition
			Expect(yaml.Unmarshal([]byte(validPolicyDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Not(Succeed()))
		})
	})

	Context("When the PolicyDefinition is valid, should create a ConfigMap", func() {
		var PolicyDefinitionName = "policy-obj"
		var namespace = "ns-plc-def-1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: PolicyDefinitionName, Namespace: namespace}}

		It("Apply PolicyDefinition", func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Apply PolicyDefinition")
			var validPolicyDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your Policy
  name: policy-obj
  namespace: ns-plc-def-1
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "core.oam.dev/v1alpha1"
          kind:       "EnvBinding"
          spec: {
            engine: parameter.engine
            appTemplate: {
              apiVersion: "core.oam.dev/v1beta1"
              kind:       "Application"
              metadata: {
                name:      context.appName
                namespace: context.namespace
              }
              spec: {
                components: context.components
              }
            }
            envs: parameter.envs
          }
        }

        #Env: {
          name: string
          patch: components: [...{
            name: string
            type: string
            properties: {...}
          }]
          placement: clusterSelector: {
            labels?: [string]: string
            name?: string
          }
        }

        parameter: {
          engine: *"ocm" | string
          envs: [...#Env]
        }
`

			var def v1beta1.PolicyDefinition
			Expect(yaml.Unmarshal([]byte(validPolicyDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)

			By("Check whether ConfigMap is created")
			var cm corev1.ConfigMap
			name := fmt.Sprintf("policy-%s%s", types.CapabilityConfigMapNamePrefix, PolicyDefinitionName)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 30*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
			Expect(cm.Labels["definition.oam.dev/name"]).Should(Equal(PolicyDefinitionName))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 30*time.Second, time.Second).Should(Equal(name))

			By("Delete the policy")
			Expect(k8sClient.Delete(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)
		})
	})

	Context("When the PolicyDefinition is invalid, should report issues", func() {
		var invalidPolicyDefinitionName = "invalid-plc1"
		var namespace = "ns-plc-def2"
		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Applying invalid PolicyDefinition", func() {
			By("Apply the PolicyDefinition")
			var invalidPolicyDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: PolicyDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your policy
  name: invalid-plc1
  namespace: ns-plc-def2
spec:
  schematic:
    cue:
      template: |
        output: {
          apiVersion: "core.oam.dev/v1alpha1"
          kind:       "EnvBinding"
          spec: {
            engine: parameter.engine
            appTemplate: {
              apiVersion: "core.oam.dev/v1beta1"
              kind:       "Application"
              metadata: {
                name:      context.appName
                namespace: context.namespace
              }
              spec: {
                components: context.components
              }
            }
            envs: parameter.envs
          }
        }

        #Env: {
          name: string
          patch: components: [...{
            name: string
            type: string
            properties: {...}
          }]
          placement: clusterSelector: {
            labels?: [string]: string
            name?: string
          }
        }

        
`

			var invalidDef v1beta1.PolicyDefinition
			Expect(yaml.Unmarshal([]byte(invalidPolicyDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			gotPolicyDefinition := &v1beta1.PolicyDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidPolicyDefinitionName, Namespace: namespace}, gotPolicyDefinition)).Should(BeNil())
		})
	})
})
