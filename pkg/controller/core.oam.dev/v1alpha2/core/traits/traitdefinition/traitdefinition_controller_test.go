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

package traitdefinition

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Apply TraitDefinition to store its schema to ConfigMap Test", func() {
	ctx := context.Background()
	var ns corev1.Namespace

	Context("When the TraitDefinition is valid, but the namespace doesn't exist, should occur errors", func() {
		It("Apply TraitDefinition", func() {
			By("Apply TraitDefinition")
			var validTraitDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  namespace: ns-tr-def
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: scaler1
spec:
  appliesToWorkloads:
    - deployments.apps
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

			var def v1beta1.TraitDefinition
			Expect(yaml.Unmarshal([]byte(validTraitDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Not(Succeed()))
		})
	})

	Context("When the TraitDefinition is valid, should create a ConfigMap", func() {
		var traitDefinitionName = "scaler1"
		var namespace = "ns-tr-def-1"
		req := reconcile.Request{NamespacedName: client.ObjectKey{Name: traitDefinitionName, Namespace: namespace}}

		It("Apply TraitDefinition", func() {
			ns = corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			By("Create a namespace")
			Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(Succeed(), &util.AlreadyExistMatcher{}))

			By("Apply TraitDefinition")
			var validTraitDefinition = `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  namespace: ns-tr-def-1
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: scaler1
spec:
  appliesToWorkloads:
    - deployments.apps
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

			var def v1beta1.TraitDefinition
			Expect(yaml.Unmarshal([]byte(validTraitDefinition), &def)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)

			By("Check whether ConfigMap is created")
			var cm corev1.ConfigMap
			name := fmt.Sprintf("trait-%s%s", types.CapabilityConfigMapNamePrefix, traitDefinitionName)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 30*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
			Expect(cm.Labels["definition.oam.dev/name"]).Should(Equal(traitDefinitionName))

			By("Check whether ConfigMapRef refer to right")
			Eventually(func() string {
				_ = k8sClient.Get(ctx, client.ObjectKey{Namespace: def.Namespace, Name: def.Name}, &def)
				return def.Status.ConfigMapRef
			}, 30*time.Second, time.Second).Should(Equal(name))

			By("Delete the trait")
			Expect(k8sClient.Delete(ctx, &def)).Should(Succeed())
			testutil.ReconcileRetry(&r, req)
		})
	})

	Context("When the TraitDefinition is invalid, should report issues", func() {
		var invalidTraitDefinitionName = "invalid-tr1"
		var namespace = "ns-tr-def2"
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
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  namespace: ns-tr-def2
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: invalid-tr1
spec:
  appliesToWorkloads:
    - deployments.apps
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

			var invalidDef v1beta1.TraitDefinition
			Expect(yaml.Unmarshal([]byte(invalidTraitDefinition), &invalidDef)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &invalidDef)).Should(Succeed())
			gotTraitDefinition := &v1beta1.TraitDefinition{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: invalidTraitDefinitionName, Namespace: namespace}, gotTraitDefinition)).Should(BeNil())
		})
	})

	Context("When the CUE Template in TraitDefinition import new added CRD", func() {
		var traitDefinitionName = "test-refresh"
		var namespace = "default"

		It("Applying TraitDefinition", func() {
			By("Create new CRD")
			newCrd := crdv1.CustomResourceDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo.example.com",
				},
				Spec: crdv1.CustomResourceDefinitionSpec{
					Group: "example.com",
					Names: crdv1.CustomResourceDefinitionNames{
						Kind:     "Foo",
						ListKind: "FooList",
						Plural:   "foo",
						Singular: "foo",
					},
					Versions: []crdv1.CustomResourceDefinitionVersion{{
						Name:         "v1",
						Served:       true,
						Storage:      true,
						Subresources: &crdv1.CustomResourceSubresources{Status: &crdv1.CustomResourceSubresourceStatus{}},
						Schema: &crdv1.CustomResourceValidation{
							OpenAPIV3Schema: &crdv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]crdv1.JSONSchemaProps{
									"spec": {
										Type:                   "object",
										XPreserveUnknownFields: pointer.BoolPtr(true),
										Properties: map[string]crdv1.JSONSchemaProps{
											"key": {Type: "string"},
										}},
									"status": {
										Type:                   "object",
										XPreserveUnknownFields: pointer.BoolPtr(true),
										Properties: map[string]crdv1.JSONSchemaProps{
											"key":      {Type: "string"},
											"app-hash": {Type: "string"},
										}}}}}},
					},
					Scope: crdv1.NamespaceScoped,
				},
			}
			Expect(k8sClient.Create(context.Background(), &newCrd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

			traitDef := `
apiVersion: core.oam.dev/v1beta1
kind: TraitDefinition
metadata:
  annotations:
    definition.oam.dev/description: "Configures replicas for your service."
  name: test-refresh
  namespace: default
spec:
  appliesToWorkloads:
    - deployments.apps
  definitionRef:
    name: foo.example.com
  schematic:
    cue:
      template: |
        output: {
          metadata: kind: "Foo"
          metadata: apiVersion: "example.com/v1"
          spec: key: parameter.key1
          status: key: parameter.key2
        }
        parameter: {
          key1: string
          key2: string
        }
`
			var td v1beta1.TraitDefinition
			Expect(yaml.Unmarshal([]byte(traitDef), &td)).Should(BeNil())
			Expect(k8sClient.Create(ctx, &td)).Should(Succeed())
			req := reconcile.Request{NamespacedName: client.ObjectKey{Name: traitDefinitionName, Namespace: namespace}}

			By("Check whether ConfigMap is created")
			var cm corev1.ConfigMap
			name := fmt.Sprintf("trait-%s%s", types.CapabilityConfigMapNamePrefix, traitDefinitionName)
			Eventually(func() bool {
				testutil.ReconcileRetry(&r, req)
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &cm)
				return err == nil
			}, 30*time.Second, time.Second).Should(BeTrue())
			Expect(cm.Data[types.OpenapiV3JSONSchema]).Should(Not(Equal("")))
			Expect(cm.Labels["definition.oam.dev/name"]).Should(Equal(traitDefinitionName))
		})
	})

})
