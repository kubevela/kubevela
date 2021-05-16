/*
 Copyright 2021. The KubeVela Authors.

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

package controllers_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("ComponentDefinition Normal tests", func() {
	ctx := context.Background()

	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		namespace = randomNamespaceName("def-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	Context("Test dynamic admission control for componentDefinition", func() {

		It("Test componentDefinition which only set type field", func() {
			workDef := &v1beta1.WorkloadDefinition{
				TypeMeta: metav1.TypeMeta{
					Kind:       "ComponentDefinition",
					APIVersion: "core.oam.dev/v1beta1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployments.apps",
				},
				Spec: v1beta1.WorkloadDefinitionSpec{
					Reference: common.DefinitionReference{
						Name:    "deployments.apps",
						Version: "v1",
					},
				},
			}
			workDef.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, workDef)).Should(BeNil())

			cd := webServiceWithNoTemplate.DeepCopy()
			cd.Spec.Workload.Definition = common.WorkloadGVK{}
			cd.Spec.Workload.Type = "deployments.apps"
			cd.SetNamespace(namespace)
			cd.SetName("test-componentdef")
			cd.Spec.Schematic.CUE.Template = webServiceV1Template
			Expect(k8sClient.Create(ctx, cd)).Should(Succeed())

			defRev := new(v1beta1.DefinitionRevision)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: "test-componentdef-v1", Namespace: namespace}, defRev)
			}).Should(BeNil())
		})

		It("Test componentDefinition only set definition field", func() {
			testCd := webServiceWithNoTemplate.DeepCopy()
			testCd.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd.SetName("test-componentdef-v1")
			testCd.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd)).Should(Succeed())

			By("check MutatingWebhook fill the type filed")
			newCd := new(v1beta1.ComponentDefinition)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: testCd.Name, Namespace: namespace}, newCd)
			}, 15*time.Second, time.Second).Should(BeNil())

			Expect(newCd.Spec.Workload.Type).Should(Equal("deployments.apps"))

			By("check workloadDefinition created by MutatingWebhook")
			newWd := new(v1beta1.WorkloadDefinition)
			wdName := newCd.Spec.Workload.Type
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: wdName, Namespace: namespace}, newWd)
			}, 15*time.Second, time.Second).Should(BeNil())

			Expect(newWd.Spec.Reference.Name).Should(Equal(wdName))
			Expect(newWd.Spec.Reference.Version).To(Equal("v1"))
		})

		It("Test componentDefinition which definition and type fields are all empty", func() {
			testCd1 := webServiceWithNoTemplate.DeepCopy()
			testCd1.SetName("test-componentdef-v2")
			testCd1.Spec.Workload.Definition = common.WorkloadGVK{}
			testCd1.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd1.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd1)).Should(HaveOccurred())
		})

		It("Test componentDefinition which definition and type point to same workload type", func() {
			testCd2 := webServiceWithNoTemplate.DeepCopy()
			testCd2.SetName("test-componentdef-v3")
			testCd2.Spec.Workload.Type = "deployments.apps"
			testCd2.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd2.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd2)).Should(Succeed())
		})

		It("Test componentDefinition which definition and type point to different workload type", func() {
			testCd3 := webServiceWithNoTemplate.DeepCopy()
			testCd3.SetName("test-componentdef-v4")
			testCd3.Spec.Workload.Type = "jobs.batch"
			testCd3.Spec.Schematic.CUE.Template = webServiceV1Template
			testCd3.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, testCd3)).Should(HaveOccurred())
		})
	})
})
