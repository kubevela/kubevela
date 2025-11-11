/*
Copyright 2024 The KubeVela Authors.

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

package e2e

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	oamcommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Application Auto update", Ordered, func() {
	ctx := context.Background()
	var k8sClient client.Client
	var namespace string
	var ns corev1.Namespace
	var err error
	var velaCommandPrefix string

	BeforeEach(func() {
		k8sClient, err = common.NewK8sClient()
		Expect(err).NotTo(HaveOccurred())

		By("Create namespace for app-autoupdate-e2e-test")
		namespace = randomNamespaceName("app-autoupdate-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		k8sClient.Create(ctx, &ns)
		velaCommandPrefix = fmt.Sprintf("vela -n %s", namespace)

	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

	It("dry-run command", func() {
		By("Create configmap-component with 1.2.0 version")
		component := configMapComponent.DeepCopy()
		component.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, component)).Should(Succeed())

		By("Execute a dry-run for application having configmap-component@v1 component")
		output, err := e2e.Exec(fmt.Sprintf("%s dry-run -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring(fmt.Sprintf(dryRunResult1, namespace)))

		By("Create application using configmap-component@v1 component")
		_, err = e2e.Exec(fmt.Sprintf("%s up -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())

		By("Create configmap-component with 1.4.0 version")
		updatedComponent := new(v1beta1.ComponentDefinition)
		updatedComponentVersion := "1.4.0"
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "configmap-component", Namespace: namespace}, updatedComponent)
			if err != nil {
				return err
			}
			updatedComponent.Spec.Schematic.CUE.Template = strings.Replace(configMapOutputTemplate, updatedComponent.Spec.Version, updatedComponentVersion, 1)
			updatedComponent.Spec.Version = updatedComponentVersion
			return k8sClient.Update(ctx, updatedComponent)
		}, 15*time.Second, time.Second).Should(BeNil())

		By("Execute a dry-run for application having configmap-component@v1 component")
		output, err = e2e.Exec(fmt.Sprintf("%s dry-run -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring(fmt.Sprintf(dryRunResult2, namespace, namespace)))
	})

	It("live-diff between application file and revision", func() {
		By("Create configmap-component with 1.2.0 version")
		component := configMapComponent.DeepCopy()
		component.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, component)).Should(Succeed())

		By("Create application using configmap-component@v1 component")
		_, err = e2e.Exec(fmt.Sprintf("%s up -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())

		By("Create configmap-component with 1.4.0 version")
		updatedComponent := new(v1beta1.ComponentDefinition)
		updatedComponentVersion := "1.4.0"
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "configmap-component", Namespace: namespace}, updatedComponent)
			if err != nil {
				return err
			}
			updatedComponent.Spec.Schematic.CUE.Template = strings.Replace(configMapOutputTemplate, updatedComponent.Spec.Version, updatedComponentVersion, 1)
			updatedComponent.Spec.Version = updatedComponentVersion
			return k8sClient.Update(ctx, updatedComponent)
		}, 15*time.Second, time.Second).Should(BeNil())

		By("Execute a live-diff command for application file and previous application")
		output, err := e2e.Exec(fmt.Sprintf("%s live-diff -f data/app.yaml -r app-with-auto-update-v1", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring(liveDiffResult))
	})

	It("live-diff between revisions", func() {
		By("Create configmap-component with 1.2.0 version")
		component := configMapComponent.DeepCopy()
		component.SetNamespace(namespace)
		Expect(k8sClient.Create(ctx, component)).Should(Succeed())

		By("Create application using configmap-component@v1 component")
		_, err = e2e.Exec(fmt.Sprintf("%s up -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())

		By("Create configmap-component with 1.4.0 version")
		updatedComponent := new(v1beta1.ComponentDefinition)
		updatedComponentVersion := "1.4.0"
		Eventually(func() error {
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "configmap-component", Namespace: namespace}, updatedComponent)
			if err != nil {
				return err
			}
			updatedComponent.Spec.Schematic.CUE.Template = strings.Replace(configMapOutputTemplate, updatedComponent.Spec.Version, updatedComponentVersion, 1)
			updatedComponent.Spec.Version = updatedComponentVersion
			return k8sClient.Update(ctx, updatedComponent)
		}, 15*time.Second, time.Second).Should(BeNil())

		By("Create application using configmap-component@v1 component")
		_, err = e2e.Exec(fmt.Sprintf("%s up -f data/app.yaml", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())

		By("Execute a live-diff command for previous two application versions")
		output, err := e2e.Exec(fmt.Sprintf("%s live-diff --revision app-with-auto-update-v2,app-with-auto-update-v1", velaCommandPrefix))
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(ContainSubstring("Application (app-with-auto-update) has no change"))
	})

})

var configMapComponent = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "configmap-component",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Version: "1.2.0",
		Schematic: &oamcommon.Schematic{
			CUE: &oamcommon.CUE{
				Template: configMapOutputTemplate,
			},
		},
	},
}

var configMapOutputTemplate = `output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "comptest"
		data: {
			expectedVersion:    "1.2.0"
		}
	}`

func randomNamespaceName(basic string) string {
	return fmt.Sprintf("%s-%s", basic, strconv.FormatInt(rand.Int63(), 16))
}

var dryRunResult1 = `---
# Application(app-with-auto-update) -- Component(test) 
---

apiVersion: v1
data:
  expectedVersion: 1.2.0
kind: ConfigMap
metadata:
  annotations:
    app.oam.dev/autoUpdate: "true"
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: test
    app.oam.dev/name: app-with-auto-update
    app.oam.dev/namespace: %[1]s
    app.oam.dev/resourceType: WORKLOAD
    workload.oam.dev/type: configmap-component-v1
  name: comptest
  namespace: %[1]s

---`

var dryRunResult2 = `---
# Application(app-with-auto-update) -- Component(test) 
---

apiVersion: v1
data:
  expectedVersion: 1.4.0
kind: ConfigMap
metadata:
  annotations:
    app.oam.dev/autoUpdate: "true"
  labels:
    app.oam.dev/appRevision: ""
    app.oam.dev/component: test
    app.oam.dev/name: app-with-auto-update
    app.oam.dev/namespace: %[1]s
    app.oam.dev/resourceType: WORKLOAD
    workload.oam.dev/type: configmap-component-v1
  name: comptest
  namespace: %[1]s

---


`

var liveDiffResult = `
-   expectedVersion: 1.2.0
+   expectedVersion: 1.4.0
`
