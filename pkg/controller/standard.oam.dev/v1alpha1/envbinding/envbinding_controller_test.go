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

package envbinding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ocmclusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	commontype "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/testutil"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("EnvBinding Normal tests", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var spokeClusterName string
	var AppTemplate v1beta1.Application
	var BaseEnvBinding v1alpha1.EnvBinding

	AppTemplate = v1beta1.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "template-app",
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
				{
					Name: "server",
					Type: "webservice",
					Properties: util.Object2RawExtension(map[string]interface{}{
						"image": "nginx",
						"port":  80,
					}),
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
					}, {
						Name: "server",
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"port": 8080,
						}),
					}},
				},
				Placement: commontype.ClusterPlacement{
					ClusterSelector: &commontype.ClusterSelector{},
				},
			}},
		},
	}

	BeforeEach(func() {
		spokeClusterName = "cluster1"
		namespace = randomNamespaceName("envbinding-unit-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		webServiceDef := webService.DeepCopy()
		webServiceDef.SetNamespace(namespace)
		Eventually(func() error {
			return k8sClient.Create(ctx, webServiceDef)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		labelsDef := labels.DeepCopy()
		labelsDef.SetNamespace(namespace)
		Eventually(func() error {
			return k8sClient.Create(ctx, labelsDef)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		podInfoDef := podInfo.DeepCopy()
		podInfoDef.SetNamespace(namespace)
		Eventually(func() error {
			return k8sClient.Create(ctx, podInfoDef)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1alpha1.EnvBinding{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &ocmclusterv1alpha1.Placement{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &ocmclusterv1alpha1.PlacementDecision{}, client.InNamespace(namespace))

		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())
	})

	Context("Test EnvBinding with OCM engine", func() {
		It("Test EnvBinding select cluster by name", func() {
			envBinding := BaseEnvBinding.DeepCopy()
			appTemplate := AppTemplate.DeepCopy()
			appTemplate.SetName("app-with-two-components")
			appTemplate.SetNamespace(namespace)

			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-select-cluster-by-name")
			envBinding.Spec.AppTemplate = v1alpha1.AppTemplate{
				RawExtension: util.Object2RawExtension(appTemplate),
			}
			envBinding.Spec.Envs[0].Placement.ClusterSelector.Name = spokeClusterName

			req := reconcile.Request{NamespacedName: client.ObjectKey{Namespace: namespace, Name: envBinding.Name}}
			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			By("Check whether the parameter is patched")
			mw1 := new(ocmworkv1.ManifestWork)
			mw1Yaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mw1Yaml), mw1)).Should(BeNil())
			workload1 := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw1.Spec.Workload.Manifests[0].Raw, workload1)).Should(BeNil())
			Expect(workload1.Spec.Template.GetLabels()["hello"]).Should(Equal("patch"))
			Expect(workload1.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))

			mw2 := new(ocmworkv1.ManifestWork)
			mw2Yaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[1].Name)]
			Expect(yaml.Unmarshal([]byte(mw2Yaml), mw2)).Should(BeNil())
			workload2 := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw2.Spec.Workload.Manifests[0].Raw, workload2)).Should(BeNil())
			Expect(workload2.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).Should(Equal(int32(8080)))

			By("Check whether the cluster is selected correctly")
			Expect(mw1.GetNamespace()).Should(Equal(spokeClusterName))
			Expect(mw2.GetNamespace()).Should(Equal(spokeClusterName))
		})

		It("Test EnvBinding select cluster by label", func() {
			envBinding := BaseEnvBinding.DeepCopy()
			appTemplate := AppTemplate.DeepCopy()
			appTemplate.SetNamespace(namespace)
			appTemplate.SetName("app-with-two-components")

			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-select-cluster-by-label")
			envBinding.Spec.AppTemplate = v1alpha1.AppTemplate{
				RawExtension: util.Object2RawExtension(appTemplate),
			}
			envBinding.Spec.Envs[0].Placement.ClusterSelector.Labels = map[string]string{
				"purpose": "test",
			}
			req := reconcile.Request{NamespacedName: client.ObjectKey{Namespace: namespace, Name: envBinding.Name}}

			plName := fmt.Sprintf("%s-%s", appTemplate.Name, envBinding.Spec.Envs[0].Name)
			Expect(fakePlacementDecision(ctx, plName, appTemplate.Namespace, spokeClusterName)).Should(BeNil())

			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			By("Check whether the parameter is patched")
			mw := new(ocmworkv1.ManifestWork)
			mwYaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mwYaml), mw)).Should(BeNil())
			workload := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw.Spec.Workload.Manifests[0].Raw, workload)).Should(BeNil())
			Expect(workload.Spec.Template.GetLabels()["hello"]).Should(Equal("patch"))
			Expect(workload.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))

			By("Check whether the cluster is selected correctly")
			Expect(mw.GetNamespace()).Should(Equal(spokeClusterName))
		})

		It("Test EnvBinding contains two envs config", func() {
			appTemplate := AppTemplate.DeepCopy()
			appTemplate.SetName("app-with-two-component")
			appTemplate.SetNamespace(namespace)

			envBinding := BaseEnvBinding.DeepCopy()
			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-with-two-env-config")
			envBinding.Spec.AppTemplate = v1alpha1.AppTemplate{
				RawExtension: util.Object2RawExtension(appTemplate),
			}
			envBinding.Spec.Envs[0].Placement.ClusterSelector.Name = spokeClusterName

			envBinding.Spec.Envs = append(envBinding.Spec.Envs, v1alpha1.EnvConfig{
				Name: "test",
				Patch: v1alpha1.EnvPatch{
					Components: []commontype.ApplicationComponent{{
						Name: "web",
						Type: "webservice",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx:1.20",
						}),
						Traits: []commontype.ApplicationTrait{
							{
								Type: "labels",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"hello": "patch-test",
								}),
							},
						},
					}},
				},
				Placement: commontype.ClusterPlacement{
					ClusterSelector: &commontype.ClusterSelector{
						Name: spokeClusterName,
					},
				},
			})

			req := reconcile.Request{NamespacedName: client.ObjectKey{Namespace: namespace, Name: envBinding.Name}}
			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			By("Check whether the parameter is patched")
			mw1 := new(ocmworkv1.ManifestWork)
			mw1Yaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mw1Yaml), mw1)).Should(BeNil())
			workload1 := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw1.Spec.Workload.Manifests[0].Raw, workload1)).Should(BeNil())
			Expect(workload1.Spec.Template.GetLabels()["hello"]).Should(Equal("patch"))
			Expect(workload1.Spec.Template.Spec.Containers[0].Image).Should(Equal("busybox"))

			mw2 := new(ocmworkv1.ManifestWork)
			mw2Yaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[1].Name, envBinding.Spec.Envs[1].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mw2Yaml), mw2)).Should(BeNil())
			workload2 := new(v1.Deployment)
			Expect(yaml.Unmarshal(mw2.Spec.Workload.Manifests[0].Raw, workload2)).Should(BeNil())
			Expect(workload2.Spec.Template.GetLabels()["hello"]).Should(Equal("patch-test"))
			Expect(workload2.Spec.Template.Spec.Containers[0].Image).Should(Equal("nginx:1.20"))
		})

		It("Test Application contains helm type component", func() {
			appTemplate := AppTemplate.DeepCopy()
			appTemplate.SetName("app-with-helm")
			appTemplate.SetNamespace(namespace)
			appTemplate.Spec.Components = []commontype.ApplicationComponent{{
				Name: "demo-podinfo",
				Type: "pod-info",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"image": map[string]string{
						"tag": "5.1.2",
					},
				}),
			}}

			envBinding := BaseEnvBinding.DeepCopy()
			envBinding.SetNamespace(namespace)
			envBinding.SetName("envbinding-with-app-has-helm")
			envBinding.Spec.AppTemplate = v1alpha1.AppTemplate{
				RawExtension: util.Object2RawExtension(appTemplate),
			}

			envBinding.Spec.Envs = []v1alpha1.EnvConfig{{
				Name: "prod",
				Patch: v1alpha1.EnvPatch{
					Components: []commontype.ApplicationComponent{{
						Name: "demo-podinfo",
						Type: "pod-info",
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": map[string]string{
								"tag": "5.1.2",
							},
						}),
					}},
				},
				Placement: commontype.ClusterPlacement{
					ClusterSelector: &commontype.ClusterSelector{
						Name: spokeClusterName,
					},
				},
			}}

			req := reconcile.Request{NamespacedName: client.ObjectKey{Namespace: namespace, Name: envBinding.Name}}
			By("Create envBinding")
			Expect(k8sClient.Create(ctx, envBinding)).Should(BeNil())
			testutil.ReconcileRetry(&r, req)

			By("Check whether create configmap")
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKey{Name: envBinding.Name, Namespace: namespace}, cm)
			}, 30*time.Second, 1*time.Second).Should(BeNil())

			mw := new(ocmworkv1.ManifestWork)
			mwYaml := cm.Data[fmt.Sprintf("%s-%s", envBinding.Spec.Envs[0].Name, envBinding.Spec.Envs[0].Patch.Components[0].Name)]
			Expect(yaml.Unmarshal([]byte(mwYaml), mw)).Should(BeNil())
			Expect(len(mw.Spec.Workload.Manifests)).Should(Equal(3))
		})
	})

})

func fakePlacementDecision(ctx context.Context, plName, namespace, clusterName string) error {
	pd := &ocmclusterv1alpha1.PlacementDecision{}
	pdName := plName + "-placement-decision"
	pd.SetName(pdName)
	pd.SetNamespace(namespace)
	pd.Status.Decisions = []ocmclusterv1alpha1.ClusterDecision{{
		ClusterName: clusterName,
	}}
	pd.SetLabels(map[string]string{
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
	if err = k8sClient.Create(ctx, pd); err != nil {
		return err
	}
	if err = k8sClient.Get(ctx, client.ObjectKey{Name: pdName, Namespace: namespace}, pd); err != nil {
		return err
	}

	return k8sClient.Status().Update(ctx, pd)
}

var webService = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "webservice",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Workload: commontype.WorkloadTypeDescriptor{
			Definition: commontype.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		},
		Schematic: &commontype.Schematic{
			CUE: &commontype.CUE{
				Template: webServiceTemplate,
			},
		},
	},
}

var webServiceTemplate = `output: {
	apiVersion: "apps/v1"
	kind:       "Deployment"
	metadata: labels: {
		"componentdefinition.oam.dev/version": "v1"
	}
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
					if parameter["env"] != _|_ {
						env: parameter.env
					}
					if context["config"] != _|_ {
						env: context.config
					}
					ports: [{
						containerPort: parameter.port
					}]
					if parameter["cpu"] != _|_ {
						resources: {
							limits:
								cpu: parameter.cpu
							requests:
								cpu: parameter.cpu
						}
					}
				}]
		}
		}
	}
}
parameter: {
	image: string
	cmd?: [...string]
	port: *80 | int
	env?: [...{
		name:   string
		value?: string
		valueFrom?: {
			secretKeyRef: {
				name: string
				key:  string
			}
		}
	}]
	cpu?: string
}
`

var labels = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "labels",
	},
	Spec: v1beta1.TraitDefinitionSpec{
		Schematic: &commontype.Schematic{
			CUE: &commontype.CUE{
				Template: labelsTemplate,
			},
		},
	},
}

var labelsTemplate = `patch: {
	spec: template: metadata: labels: {
		for k, v in parameter {
			"\(k)": v
		}
	}
}
parameter: [string]: string
`

var podInfo = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "pod-info",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Workload: commontype.WorkloadTypeDescriptor{
			Definition: commontype.WorkloadGVK{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
			},
		},
		Schematic: &commontype.Schematic{
			HELM: &commontype.Helm{
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
		},
	},
}
