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

package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Application AutoUpdate", func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace

	BeforeEach(func() {
		By("Create namespace for app-autoupdate-e2e-test")
		namespace = randomNamespaceName("app-autoupdate-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	AfterEach(func() {
		By("Clean up resources after a test")
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.TraitDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

	renderedVersion := func(g Gomega) string {
		cm := new(corev1.ConfigMap)
		g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)).To(Succeed())
		return cm.Data["expectedVersion"]
	}

	replicas := func(g Gomega) int32 {
		return deploymentReplicas(ctx, g, namespace, "webservice-component")
	}

	Context("Enabled", func() {
		It("When specified exact component version available. App should use exact specified version.", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "1.0.0")
			publishComponentVersion(ctx, namespace, componentType, "1.2.0")

			By("Create application using configmap-component@v1.0.0")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.0.0")
			createAutoUpdateApp(ctx, app)
			Eventually(renderedVersion, 15*time.Second, 250*time.Millisecond).Should(Equal("1.0.0"))

			publishComponentVersion(ctx, namespace, componentType, "1.4.0")

			By("The app stays on the exact version across reconciles")
			ConsistentlyReconciled(ctx, app, func(g Gomega) {
				g.Expect(renderedVersion(g)).To(Equal("1.0.0"))
			}).WithTimeout(5 * time.Second).Should(Succeed())
		})

		It("When new component version release after app creation, app should use new version during reconciliation", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "2.2.0")
			publishComponentVersion(ctx, namespace, componentType, "2.3.0")

			By("Create application using configmap-component@v2")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "2")
			createAutoUpdateApp(ctx, app)
			Eventually(renderedVersion, 15*time.Second, 250*time.Millisecond).Should(Equal("2.3.0"))

			publishComponentVersion(ctx, namespace, componentType, "2.4.0")

			By("The app moves to the new version on its next reconcile")
			EventuallyReconciled(ctx, app, func(g Gomega) {
				g.Expect(renderedVersion(g)).To(Equal("2.4.0"))
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})

		It("When speicified version is available for one component and unavailable for other, app should use autoupdate the latter", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "1.4.5")

			By("Create application using configmap-component@v1.4 beside webservice@v1")
			app := updateAppComponent(appWithTwoComponentTemplate, "app1", namespace, componentType, "first-component", "1.4")
			createAutoUpdateApp(ctx, app)
			Eventually(renderedVersion, 15*time.Second, 250*time.Millisecond).Should(Equal("1.4.5"))
			Eventually(func(g Gomega) {
				g.Expect(deploymentReplicas(ctx, g, namespace, "second-component")).To(BeEquivalentTo(1))
			}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
		})

		It("When specified exact trait version available, app should use exact version", func() {
			traitType := "scaler-trait"
			createTraitVersion(ctx, namespace, traitType, "1.0.0", "1")
			publishTraitVersion(ctx, namespace, traitType, "1.2.0", "2")

			By("Create application using scaler-trait@v1.2.0")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.2.0")
			createAutoUpdateApp(ctx, app)
			Eventually(replicas, 30*time.Second, 250*time.Millisecond).Should(BeEquivalentTo(2))

			publishTraitVersion(ctx, namespace, traitType, "1.4.0", "3")

			By("The app stays on the exact version across reconciles")
			ConsistentlyReconciled(ctx, app, func(g Gomega) {
				g.Expect(replicas(g)).To(BeEquivalentTo(2))
			}).WithTimeout(5 * time.Second).Should(Succeed())
		})

		It("When new trait version is created after app creation, app should use new version during reconciliation", func() {
			traitType := "scaler-trait"
			createTraitVersion(ctx, namespace, traitType, "1.4.5", "4")

			By("Create application using scaler-trait@v1.4")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.4")
			createAutoUpdateApp(ctx, app)
			Eventually(replicas, 30*time.Second, 250*time.Millisecond).Should(BeEquivalentTo(4))

			publishTraitVersion(ctx, namespace, traitType, "1.4.8", "2")

			By("The app moves to the new version on its next reconcile")
			EventuallyReconciled(ctx, app, func(g Gomega) {
				g.Expect(replicas(g)).To(BeEquivalentTo(2))
			}).WithTimeout(30 * time.Second).Should(Succeed())
		})

		It("When Autoupdate and Publish version annotation are specified in application, app creation should fail", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "1.4.5")

			By("Create application using configmap-component@v1.4.5")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.4.5")
			app.ObjectMeta.Annotations[oam.AnnotationPublishVersion] = "alpha"
			err := k8sClient.Create(ctx, app)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("Application has both autoUpdate and publishVersion annotations. Only one can be present"))
		})
	})

	Context("Disabled", func() {
		It("When specified component version is available, app should use specified version", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "1.0.0")
			publishComponentVersion(ctx, namespace, componentType, "1.2.0")

			By("Create application using configmap-component@v1.0.0")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.0.0")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			createAutoUpdateApp(ctx, app)
			Eventually(renderedVersion, 15*time.Second, 250*time.Millisecond).Should(Equal("1.0.0"))
		})

		It("When specified component version is unavailable, app creation should fail", func() {
			componentType := "configmap-component"
			createComponentVersion(ctx, namespace, componentType, "1.2.0")

			By("Create application using configmap-component@v1")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).ShouldNot(Succeed())
		})

		It("When specified trait version is available, app should specified trait version", func() {
			traitType := "scaler-trait"
			createTraitVersion(ctx, namespace, traitType, "1.0.0", "1")
			publishTraitVersion(ctx, namespace, traitType, "1.2.0", "2")

			By("Create application using scaler-trait@v1.0.0")
			app := updateAppTrait(traitApp, "app1", namespace, traitType, "1.0.0")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			createAutoUpdateApp(ctx, app)
			Eventually(replicas, 30*time.Second, 250*time.Millisecond).Should(BeEquivalentTo(1))

			publishTraitVersion(ctx, namespace, traitType, "1.4.0", "3")

			By("The app stays on the specified version across reconciles")
			ConsistentlyReconciled(ctx, app, func(g Gomega) {
				g.Expect(replicas(g)).To(BeEquivalentTo(1))
			}).WithTimeout(5 * time.Second).Should(Succeed())
		})
	})
})

// TODO Add test cases for policydefinition and worflowstepdefinition

func updateAppComponent(appTemplate v1beta1.Application, appName, namespace, typeName, componentName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)

	app.Spec.Components[0].Name = componentName
	return app
}

func updateAppTrait(traitApp v1beta1.Application, appName, namespace, typeName, traitVersion string) *v1beta1.Application {
	app := traitApp.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Traits[0].Type = fmt.Sprintf("%s@v%s", typeName, traitVersion)

	return app
}

// createComponentVersion creates a ComponentDefinition whose ConfigMap output
// records version, and waits for its revision.
func createComponentVersion(ctx context.Context, namespace, name, version string) {
	By(fmt.Sprintf("Create %s with %s version", name, version))
	Expect(k8sClient.Create(ctx, createComponent(version, namespace, name))).Should(Succeed())
	waitForDefinitionRevision(ctx, namespace, name, version)
}

// publishComponentVersion moves an existing ComponentDefinition to version,
// rendering that version, and waits for the new revision.
func publishComponentVersion(ctx context.Context, namespace, name, version string) {
	By(fmt.Sprintf("Publish %s with %s version", name, version))
	def := new(v1beta1.ComponentDefinition)
	Eventually(func() error {
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, def); err != nil {
			return err
		}
		def.Spec.Version = version
		def.Spec.Schematic.CUE.Template = createOutputConfigMap(version)
		return k8sClient.Update(ctx, def)
	}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	waitForDefinitionRevision(ctx, namespace, name, version)
}

// createTraitVersion creates a scaler TraitDefinition that sets replicas, and
// waits for its revision.
func createTraitVersion(ctx context.Context, namespace, name, version, replicas string) {
	By(fmt.Sprintf("Create %s with %s version and %s replicas", name, version, replicas))
	Expect(k8sClient.Create(ctx, createTrait(version, namespace, name, replicas))).Should(Succeed())
	waitForDefinitionRevision(ctx, namespace, name, version)
}

// publishTraitVersion moves an existing scaler TraitDefinition to version with
// a new replica count, and waits for the new revision.
func publishTraitVersion(ctx context.Context, namespace, name, version, replicas string) {
	By(fmt.Sprintf("Publish %s with %s version and %s replicas", name, version, replicas))
	def := new(v1beta1.TraitDefinition)
	Eventually(func() error {
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, def); err != nil {
			return err
		}
		def.Spec.Version = version
		def.Spec.Schematic.CUE.Template = createScalerTraitOutput(replicas)
		return k8sClient.Update(ctx, def)
	}, 15*time.Second, 250*time.Millisecond).Should(Succeed())
	waitForDefinitionRevision(ctx, namespace, name, version)
}

// waitForDefinitionRevision waits for the revision the definition controller
// mints for version, which is what an app's @vX reference resolves against.
func waitForDefinitionRevision(ctx context.Context, namespace, name, version string) {
	key := client.ObjectKey{Name: fmt.Sprintf("%s-v%s", name, version), Namespace: namespace}
	Eventually(func() error {
		return k8sClient.Get(ctx, key, &v1beta1.DefinitionRevision{})
	}, 30*time.Second, 250*time.Millisecond).Should(Succeed(), "no DefinitionRevision %s", key.Name)
}

// createAutoUpdateApp creates app, retrying only the webhook's failure to find
// a definition revision. The webhook lists revisions through its cache, which
// can trail the revision the test has just read from the API server.
func createAutoUpdateApp(ctx context.Context, app *v1beta1.Application) {
	Eventually(func() error {
		err := k8sClient.Create(ctx, app.DeepCopy())
		if err != nil && !strings.Contains(err.Error(), "error finding definition revision") {
			return StopTrying("create refused").Wrap(err)
		}
		return err
	}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
}

// deploymentReplicas reads the desired replicas the scaler trait wrote. Pod
// counts lag it while old pods terminate.
func deploymentReplicas(ctx context.Context, g Gomega, namespace, name string) int32 {
	deploy := &appsv1.Deployment{}
	g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, deploy)).To(Succeed())
	g.Expect(deploy.Spec.Replicas).NotTo(BeNil())
	return *deploy.Spec.Replicas
}

func createComponent(componentVersion, namespace, name string) *v1beta1.ComponentDefinition {
	component := configMapComponent.DeepCopy()
	component.ObjectMeta.Name = name
	component.Spec.Version = componentVersion
	component.Spec.Schematic.CUE.Template = createOutputConfigMap(componentVersion)
	component.SetNamespace(namespace)
	return component
}

func createTrait(traitVersion, namespace, name, replicas string) *v1beta1.TraitDefinition {
	trait := scalerTrait.DeepCopy()
	trait.ObjectMeta.Name = name
	trait.Spec.Version = traitVersion
	trait.Spec.Schematic.CUE.Template = createScalerTraitOutput(replicas)
	trait.SetNamespace(namespace)
	return trait
}

func createScalerTraitOutput(replicas string) string {
	return strings.Replace(scalerTraitOutputTemplate, "1", replicas, 1)
}

func createOutputConfigMap(toVersion string) string {
	return strings.Replace(configMapOutputTemplate, "1.0.0", toVersion, 1)
}

var configMapComponent = &v1beta1.ComponentDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "configmap-component",
	},
	Spec: v1beta1.ComponentDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}
var configMapOutputTemplate = `output: {
		apiVersion: "v1"
		kind:       "ConfigMap"
		metadata: name: "comptest"
		data: {
			expectedVersion:    "1.0.0"
		}
	}`

var appTemplate = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "Name",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name: "comp1Name",
				Type: "type",
			},
		},
	},
}

var appWithTwoComponentTemplate = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "Name",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name: "first-component",
				Type: "configmap-component",
			},
			{
				Name: "second-component",
				Type: "webservice@v1",
				Properties: util.Object2RawExtension(map[string]interface{}{
					"image": "nginx",
				}),
			},
		},
	},
}

var scalerTraitOutputTemplate = `patch: spec: replicas: 1`

var scalerTrait = &v1beta1.TraitDefinition{
	TypeMeta: metav1.TypeMeta{
		Kind:       "TraitDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name:        "scaler-trait",
		Annotations: map[string]string{},
	},
	Spec: v1beta1.TraitDefinitionSpec{
		Version: "1.0.0",
		Schematic: &common.Schematic{
			CUE: &common.CUE{
				Template: "",
			},
		},
	},
}

var traitApp = v1beta1.Application{
	ObjectMeta: metav1.ObjectMeta{
		Name:      "app-with-trait",
		Namespace: "Namespace",
		Annotations: map[string]string{
			oam.AnnotationAutoUpdate: "true",
		},
	},
	Spec: v1beta1.ApplicationSpec{
		Components: []common.ApplicationComponent{
			{
				Name:       "webservice-component",
				Type:       "webservice",
				Properties: &runtime.RawExtension{Raw: []byte(`{"image": "busybox"}`)},
				Traits: []common.ApplicationTrait{
					{
						Type: "scaler-trait",
					},
				},
			},
		},
	},
}
