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
	"encoding/json"
	"fmt"
	"time"

	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commontype "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	ocmapi "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1alpha2/envbinding/api"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("EnvBinding Normal tests", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var spokeClusterName string
	var AppTemplate v1beta1.Application
	var BaseEnvBinding v1alpha1.EnvBinding

	BeforeEach(func() {
		spokeClusterName = "cluster1"
		namespace = randomNamespaceName("envbinding-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		AppTemplate = v1beta1.Application{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Application",
				APIVersion: "core.oam.dev/v1beta1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "template-app",
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []commontype.ApplicationComponent{
					{
						Name: "web",
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx",
						}),
						Traits: []commontype.ApplicationTrait{
							{
								Type: "labels",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"hello": "world",
								}),
							},
						},
					},
				},
			},
		}

		BaseEnvBinding = v1alpha1.EnvBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "EnvBinding",
				APIVersion: "core.oam.dev/v1beta1",
			},
			Spec: v1alpha1.EnvBindingSpec{
				AppTemplate: v1alpha1.AppTemplate{
					RawExtension: util.Object2RawExtension(AppTemplate),
				},
				Engine: v1alpha1.OCMEngine,
				Envs: []v1alpha1.EnvConfig{{
					Name: "prod",
					Patch: v1alpha1.EnvPatch{
						Components: []commontype.ApplicationComponent{{
							Name: "web",
							Type: "webservice",
							Properties: util.Object2RawExtension(map[string]interface{}{
								"image": "busybox",
							}),
							Traits: []commontype.ApplicationTrait{
								{
									Type: "labels",
									Properties: util.Object2RawExtension(map[string]interface{}{
										"hello": "patch",
									}),
								},
							},
						}},
					},
					Placement: commontype.ClusterPlacement{
						ClusterSelector: &commontype.ClusterSelector{},
					},
				}},
			},
		}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1alpha1.EnvBinding{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	Context("Test EnvBinding with OCM engine", func() {
		It("Test EnvBinding select cluster by name", func() {
			envBinding := BaseEnvBinding.DeepCopy()
			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-select-cluster-by-name")
			envBinding.Spec.Envs[0].Placement.ClusterSelector.Name = spokeClusterName

			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			By("Check whether the parameter is patched")
			mw := new(ocmapi.ManifestWork)
			mwYaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mwYaml), mw)).Should(BeNil())
			workload := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, workload)).Should(BeNil())
			Expect(workload.Spec.Template.GetLabels()["hello"]).Should(Equal("patch"))
			Expect(workload.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))

			By("Check whether the cluster is selected correctly")
			Expect(mw.GetNamespace()).Should(Equal(spokeClusterName))
		})

		It("Test EnvBinding select cluster by label", func() {
			envBinding := BaseEnvBinding.DeepCopy()
			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-select-cluster-by-label")
			envBinding.Spec.Envs[0].Placement.ClusterSelector.Labels = map[string]string{
				"purpose": "test",
			}

			plName := fmt.Sprintf("%s-%s", AppTemplate.Name, envBinding.Spec.Envs[0].Name)
			Expect(fakePlacementDecision(ctx, plName, namespace, spokeClusterName)).Should(BeNil())

			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			By("Check whether the parameter is patched")
			mw := new(ocmapi.ManifestWork)
			mwYaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mwYaml), mw)).Should(BeNil())
			workload := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, workload)).Should(BeNil())
			Expect(workload.Spec.Template.GetLabels()["hello"]).Should(Equal("patch"))
			Expect(workload.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))

			By("Check whether the cluster is selected correctly")
			Expect(mw.GetNamespace()).Should(Equal(spokeClusterName))
		})
	})

})

func fakePlacementDecision(ctx context.Context, plName, namespace, clusterName string) error {
	PlacementDecisionGVK := schema.GroupVersionKind{
		Group:   "cluster.open-cluster-management.io",
		Version: "v1alpha1",
		Kind:    "PlacementDecision",
	}
	pd := &ocmapi.PlacementDecision{}
	pdName := plName + "-placement-decision"
	unstructuredPd := common.GenerateUnstructuredObj(pdName, namespace, PlacementDecisionGVK)
	pd.Status.Decisions = []ocmapi.ClusterDecision{{
		ClusterName: clusterName,
	}}

	unstructuredPd.SetLabels(map[string]string{
		"cluster.open-cluster-management.io/placement": plName,
	})

	bts, err := json.Marshal(pd.Status)
	if err != nil {
		return err
	}
	data := make(map[string]interface{})
	if err = json.Unmarshal(bts, &data); err != nil {
		return err
	}
	if err = k8sClient.Create(ctx, unstructuredPd); err != nil {
		return err
	}
	if err = k8sClient.Get(ctx, client.ObjectKey{Name: pdName, Namespace: namespace}, unstructuredPd); err != nil {
		return err
	}
	if err = unstructured.SetNestedMap(unstructuredPd.Object, data, "status"); err != nil {
		return err
	}
	return k8sClient.Status().Update(ctx, unstructuredPd)
}
