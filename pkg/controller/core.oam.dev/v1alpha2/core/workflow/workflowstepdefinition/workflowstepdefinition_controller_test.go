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

package workflowstepdefinition

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

var _ = Describe("Apply WorkflowStepDefinition to store its schema to ConfigMap Test", func() {
	ctx := context.Background()
	var ns corev1.Namespace

	Context("When the WorkflowStepDefinition is valid, but the namespace doesn't exist, should occur errors", func() {
		It("Apply WorkflowStepDefinition", func() {
			By("Apply WorkflowStepDefinition")
			var validWorkflowStepDefinition = `
apiVersion: core.oam.dev/v1beta1
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your workflow steps
  name: apply-object
  namespace: not-exist
spec:
  schematic:
    cue:
      template: |
        import (
        	"vela/op"
        )

        apply: op.#Apply & {
        	value:   parameter.value
        	cluster: parameter.cluster
        }
        parameter: {
        	// +usage=Specify the value of the object
        	value: {...}
        	// +usage=Specify the cluster of the object
        	cluster: *"" | string
        }
`

			var def v1beta1.WorkflowStepDefinition
			Expect(yaml.Unmarshal([]byte(validWorkflowStepDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Not(Succeed()))
		})
	})

	Context("When the WorkflowStepDefinition is valid, should create a ConfigMap", func() {
		var WorkflowStepDefinitionName = "apply-object"
		var namespace = "ns-wfs-def-1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: WorkflowStepDefinitionName, Namespace: namespace}}

		It("Apply WorkflowStepDefinition", func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Apply WorkflowStepDefinition")
			var validWorkflowStepDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your workflow steps
  name: apply-object
  namespace: ns-wfs-def-1
spec:
  schematic:
    cue:
      template: |
        import (
        	"vela/op"
        )

        apply: op.#Apply & {
        	value:   parameter.value
        	cluster: parameter.cluster
        }
        parameter: {
        	// +usage=Specify the value of the object
        	value: {...}
        	// +usage=Specify the cluster of the object
        	cluster: *"" | string
        }
`

			var def v1beta1.WorkflowStepDefinition
			Expect(yaml.Unmarshal([]byte(validWorkflowStepDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)

			By("Check whether ConfigMap is created")
			var cm corev1.ConfigMap
			name := fmt.Sprintf("workflowstep-%s%s", types.CapabilityConfigMapNamePrefix, WorkflowStepDefinitionName)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 30*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
			Expect(cm.Labels["definition.oam.dev/name"]).Should(Equal(WorkflowStepDefinitionName))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 30*time.Second, time.Second).Should(Equal(name))

			By("Delete the workflowstep")
			Expect(k8sClient.Delete(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)
		})
	})

	Context("When the WorkflowStepDefinition is invalid, should report issues", func() {
		var invalidWorkflowStepDefinitionName = "invalid-wf1"
		var namespace = "ns-wfs-def2"
		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Applying invalid WorkflowStepDefinition", func() {
			By("Apply the WorkflowStepDefinition")
			var invalidWorkflowStepDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: WorkflowStepDefinition
metadata:
  annotations:
    definition.oam.dev/description: Apply raw kubernetes objects for your workflow steps
  name: invalid-wf1
  namespace: ns-wfs-def2
spec:
  schematic:
    cue:
      template: |
        import (
        	"vela/op"
        )

        apply: op.#Apply & {
        	value:   parameter.value
        	cluster: parameter.cluster
        }
`

			var invalidDef v1beta1.WorkflowStepDefinition
			Expect(yaml.Unmarshal([]byte(invalidWorkflowStepDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			gotWorkflowStepDefinition := &v1beta1.WorkflowStepDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidWorkflowStepDefinitionName, Namespace: namespace}, gotWorkflowStepDefinition)).Should(BeNil())
		})
	})
})
