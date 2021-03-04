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

package traitdefinition

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

var _ = Describe("Apply TraitDefinition to store its schema to ConfigMap Test", func() {
	ctx := context.Background()
	var (
		namespace = types.DefaultKubeVelaNS
		ns        corev1.Namespace
	)

	Context("When the TraitDefinition is valid, should create a ConfigMap", func() {
		var traitDefinitionName = "scaler1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: traitDefinitionName}}

		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Apply TraitDefinition", func() {
			By("Apply TraitDefinition")
			var validTraitDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: scaler1
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  schematic:
    cue:
      template: |
        outputs: scaler: {
        	apiVersion: "core.oam.dev/v1alpha2"
        	kind:       "ManualScalerTrait"
        	spec: {
        		replicaCount: parameter.replicas
        	}
        }
        parameter: {
        	//+short=r
        	//+usage=Replicas of the workload
        	replicas: *1 | int
        }
`

			var def v1alpha2.TraitDefinition
			Expect(yaml.Unmarshal([]byte(validTraitDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())

			By("Check whether ConfigMap is created")
			reconcileRetry(&r, req)
			var cm corev1.ConfigMap
			name := fmt.Sprintf("%s%s", types.CapabilityConfigMapNamePrefix, traitDefinitionName)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: types.DefaultKubeVelaNS, Name: name}, &cm)).Should(Succeed())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
		})
	})

	Context("When the TraitDefinition is invalid, should report issues", func() {
		var invalidTraitDefinitionName = "invalid-tr1"

		BeforeEach(func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))
		})

		It("Applying invalid TraitDefinition", func() {
			By("Apply the TraitDefinition")
			var invalidTraitDefinition = `
apiVersion: core.oam.dev/v1alpha2
kind: TraitDefinition
metadata:
  namespace: vela-system
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: invalid-tr1
spec:
  appliesToWorkloads:
    - webservice
    - worker
  definitionRef:
    name: manualscalertraits.core.oam.dev
  workloadRefPath: spec.workloadRef
  schematic:
    cue:
      template: |
        outputs: scaler: {
        	apiVersion: "core.oam.dev/v1alpha2"
        	kind:       "ManualScalerTrait"
        	spec: {
        		replicaCount: 2
        	}
        }
`

			var invalidDef v1alpha2.TraitDefinition
			Expect(yaml.Unmarshal([]byte(invalidTraitDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			gotTraitDefinition := &v1alpha2.TraitDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidTraitDefinitionName, Namespace: namespace}, gotTraitDefinition)).Should(BeNil())
		})
	})
})
