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

package controllers_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test application containing kube module", func() {
	ctx := context.Background()
	var (
		appName  = "test-app"
		compName = "test-comp"
		cdName   = "test-kube-worker"
		wdName   = "test-kube-worker-wd"
		tdName   = "test-virtualgroup"
	)
	var namespace string
	var app v1beta1.Application
	var ns corev1.Namespace

	var testTemplate = func() runtime.RawExtension {
		yamlStr := `apiVersion: apps/v1
kind: Deployment
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
      ports:
      - containerPort: 80 `
		b, _ := yaml.YAMLToJSON([]byte(yamlStr))
		return runtime.RawExtension{Raw: b}
	}

	BeforeEach(func() {
		namespace = randomNamespaceName("kube-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(
			func() error {
				return k8sClient.Create(ctx, &ns)
			},
			time.Second*3, time.Millisecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

		cd := v1beta1.ComponentDefinition{}
		cd.SetName(cdName)
		cd.SetNamespace(namespace)
		cd.Spec.Workload.Definition = common.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}
		cd.Spec.Schematic = &common.Schematic{
			KUBE: &common.Kube{
				Template: testTemplate(),
				Parameters: []common.KubeParameter{
					{
						Name:        "image",
						ValueType:   common.StringType,
						FieldPaths:  []string{"spec.template.spec.containers[0].image"},
						Required:    pointer.Bool(true),
						Description: pointer.String("test description"),
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, &cd)).Should(Succeed())

		By("Install a patch trait used to test CUE module")
		td := v1beta1.TraitDefinition{}
		td.SetName(tdName)
		td.SetNamespace(namespace)
		td.Spec.AppliesToWorkloads = []string{"deployments.apps"}
		td.Spec.Schematic = &common.Schematic{
			CUE: &common.CUE{
				Template: `patch: {
      	spec: template: {
      		metadata: labels: {
      			if parameter.type == "namespace" {
      				"app.namespace.virtual.group": parameter.group
      			}
      			if parameter.type == "cluster" {
      				"app.cluster.virtual.group": parameter.group
      			}
      		}
      	}
      }
      parameter: {
      	group: *"default" | string
      	type:  *"namespace" | string
      }`,
			},
		}
		Expect(k8sClient.Create(ctx, &td)).Should(Succeed())

		By("Verify ComponentDefinition and TraitDefinition are created successfully")
		Eventually(func() error {
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: cdName, Namespace: namespace}, &v1beta1.ComponentDefinition{}); err != nil {
				return err
			}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: tdName, Namespace: namespace}, &v1beta1.TraitDefinition{}); err != nil {
				return err
			}
			return nil
		}, 20*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Add 'deployments.apps' to scaler's appliesToWorkloads")
		scalerTd := v1beta1.TraitDefinition{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler", Namespace: "vela-system"}, &scalerTd)).Should(Succeed())
		scalerTd.Spec.AppliesToWorkloads = []string{"deployments.apps", "webservice", "worker"}
		scalerTd.SetResourceVersion("")
		Expect(k8sClient.Patch(ctx, &scalerTd, client.Merge)).Should(Succeed())
	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.WorkloadDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		By(fmt.Sprintf("Delete the entire namespaceName %s", ns.Name))
		Expect(k8sClient.Delete(ctx, &ns, client.PropagationPolicy(metav1.DeletePropagationForeground))).Should(Succeed())

		By("Remove 'deployments.apps' from scaler's appliesToWorkloads")
		scalerTd := v1beta1.TraitDefinition{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "scaler", Namespace: "vela-system"}, &scalerTd)).Should(Succeed())
		scalerTd.Spec.AppliesToWorkloads = []string{"webservice", "worker"}
		scalerTd.SetResourceVersion("")
		Expect(k8sClient.Patch(ctx, &scalerTd, client.Merge)).Should(Succeed())
	})

	It("Test deploy an application containing kube module", func() {
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: cdName,
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx:1.14.0",
						}),
						Traits: []common.ApplicationTrait{
							{
								Type: "scaler",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"replicas": 2,
								}),
							},
							{
								Type: tdName,
								Properties: util.Object2RawExtension(map[string]interface{}{
									"group": "my-group",
									"type":  "cluster",
								}),
							},
						},
					},
				},
			},
		}
		By("Create application")
		Eventually(func() error {
			return k8sClient.Create(ctx, app.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		deploy := &appsv1.Deployment{}
		deployName := compName
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
		}, 30*time.Second, 3*time.Second).Should(Succeed())

		By("Verify two traits are applied to the workload")
		Eventually(func() bool {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify patch trait is applied")
			templateLabels := deploy.Spec.Template.Labels
			if templateLabels["app.cluster.virtual.group"] != "my-group" {
				return false
			}
			By("Verify scaler trait is applied")
			if *deploy.Spec.Replicas != 2 {
				return false
			}
			By("Verify parameter is applied")
			return deploy.Spec.Template.Spec.Containers[0].Image == "nginx:1.14.0"
		}, 15*time.Second, 3*time.Second).Should(BeTrue())

		By("Update the application")
		app = v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: cdName,
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx:1.14.1", // nginx:1.14.0 => nginx:1.14.1
						}),
						Traits: []common.ApplicationTrait{
							{
								Type: "scaler",
								Properties: util.Object2RawExtension(map[string]interface{}{
									"replicas": 3, // change 2 => 3
								}),
							},
							{
								Type: tdName,
								Properties: util.Object2RawExtension(map[string]interface{}{
									"group": "my-group-0", // change my-group => my-group-0
									"type":  "cluster",
								}),
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Patch(ctx, &app, client.Merge)).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		deploy = &appsv1.Deployment{}
		deployName = compName

		By("Verify the changes are applied to the workload")
		Eventually(func() bool {
			deploy := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy); err != nil {
				return false
			}
			By("Verify new patch trait is applied")
			templateLabels := deploy.Spec.Template.Labels
			if templateLabels["app.cluster.virtual.group"] != "my-group-0" {
				return false
			}
			By("Verify new scaler trait is applied")
			if *deploy.Spec.Replicas != 3 {
				return false
			}
			By("Verify new parameter is applied")
			return deploy.Spec.Template.Spec.Containers[0].Image == "nginx:1.14.1"
		}, 60*time.Second, 10*time.Second).Should(BeTrue())
	})

	It("Test deploy an application containing kube module defined by workloadDefinition", func() {
		workloaddef := v1beta1.WorkloadDefinition{}
		workloaddef.SetName(wdName)
		workloaddef.SetNamespace(namespace)
		workloaddef.Spec.Reference = common.DefinitionReference{Name: "deployments.apps", Version: "v1"}
		workloaddef.Spec.Schematic = &common.Schematic{
			KUBE: &common.Kube{
				Template: testTemplate(),
				Parameters: []common.KubeParameter{
					{
						Name:        "image",
						ValueType:   common.StringType,
						FieldPaths:  []string{"spec.template.spec.containers[0].image"},
						Required:    pointer.Bool(true),
						Description: pointer.String("test description"),
					},
				},
			},
		}
		By("register workloadDefinition")
		Expect(k8sClient.Create(ctx, &workloaddef)).Should(Succeed())

		appTestName := "test-app-refer-to-workloaddef"
		appTest := v1beta1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      appTestName,
				Namespace: namespace,
			},
			Spec: v1beta1.ApplicationSpec{
				Components: []common.ApplicationComponent{
					{
						Name: compName,
						Type: cdName,
						Properties: util.Object2RawExtension(map[string]interface{}{
							"image": "nginx:1.14.0",
						}),
					},
				},
			},
		}
		By("Create application")
		Eventually(func() error {
			return k8sClient.Create(ctx, appTest.DeepCopy())
		}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verify the workload(deployment) is created successfully")
		deploy := &appsv1.Deployment{}
		deployName := compName
		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKey{Name: deployName, Namespace: namespace}, deploy)
		}, 15*time.Second, 3*time.Second).Should(Succeed())
	})

	It("Test store JSON schema of Kube parameter in ConfigMap", func() {
		By("Get the ConfigMap")
		cmName := fmt.Sprintf("component-schema-%s", cdName)
		Eventually(func() error {
			cm := &corev1.ConfigMap{}
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: cmName, Namespace: namespace}, cm); err != nil {
				return err
			}
			if cm.Data[types.OpenapiV3JSONSchema] == "" {
				return errors.New("json schema is not found in the ConfigMap")
			}
			return nil
		}, 60*time.Second, 5*time.Second).Should(Succeed())
	})
})
