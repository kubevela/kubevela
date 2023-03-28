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
										XPreserveUnknownFields: pointer.Bool(true),
										Properties: map[string]crdv1.JSONSchemaProps{
											"key": {Type: "string"},
										}},
									"status": {
										Type:                   "object",
										XPreserveUnknownFields: pointer.Bool(true),
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
          kind: "Foo"
          apiVersion: "example.com/v1"
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
