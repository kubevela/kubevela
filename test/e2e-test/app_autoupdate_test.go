package controllers_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/util"
)

var _ = Describe("Application AutoUpdate", Ordered, func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var sleepTime = 360 * time.Second

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
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

	Context("Enabled", func() {
		It("When specified exact component version available", func() {
			By("Create configmap-component with 1.0.0 version")
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 1.2.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-component@1.2.0")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

			By("Create configmap-component with 1.4.0 version")
			updatedComponentVersion = "1.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(componentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(sleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

		})

		It("When speicified component version is unavailable", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create application using configmap-component@1.4")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.4")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

		})

		It("When specified component version is unavailable, and after app creation new version of component is available", func() {
			By("Create configmap-component with 2.2.0 version")
			componentVersion := "2.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 2.3.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "2.3.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())
			time.Sleep(10 * time.Second)

			By("Create application using configmap-component@2")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "2")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(updatedComponentVersion))

			By("Create configmap-component with 2.4.0 version")
			updatedComponentVersion = "2.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(sleepTime)

			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(updatedComponentVersion))

		})

		It("When speicified component version is unavailable for one component and others available", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create application using configmap-component@1.4 version")
			app := updateAppComponent(appWithTwoComponentTemplate, "app1", namespace, componentType, "first-component", "1.4")

			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))

			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(1))

		})

		It("When specified exact trait version available", func() {
			By("Create scaler-trait with 1.0.0 version and 1 replica")
			traitVersion := "1.0.0"
			traitType := "scaler-trait"
			trait := createTrait(traitVersion, namespace, traitType, "1")
			trait.SetNamespace(namespace)
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create scaler-trait with 1.2.0 version and 2 replicas")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				fmt.Println(updatedTrait.Spec.Schematic.CUE.Template)
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using scaler-trait@v1.2.0")
			app := updatedAppTrait(traitApp, "app1", namespace, traitType, updatedTraitVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))

			By("Create scaler-trait with 1.4.0 version")
			updatedTraitVersion = "1.4.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("3")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(sleepTime)
			pods = new(corev1.PodList)
			opts = []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))
		})

		It("When specified trait version is unavailable", func() {
			By("Create scaler-trait with 1.4.5 version and 4 replica")
			traitVersion := "1.4.5"
			traitType := "scaler-trait"

			trait := createTrait(traitVersion, namespace, traitType, "4")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create application using scaler-trait@v1.4")
			app := updatedAppTrait(traitApp, "app1", namespace, traitType, traitVersion)
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(4))
		})

		It("When specified component version is not exact, and after app creation new version of component is available", func() {
			By("Create scaler-trait with 1.4.5 version and 4 replica")
			traitVersion := "1.4.5"
			traitType := "scaler-trait"

			trait := createTrait(traitVersion, namespace, traitType, "4")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create application using scaler-trait@v1.4")
			app := updatedAppTrait(traitApp, "app1", namespace, traitType, "1.4")
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(4))

			By("Create scaler-trait with 1.5.0 version and 2 replicas")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.4.8"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Wait for application to reconcile")
			time.Sleep(sleepTime)
			pods = new(corev1.PodList)
			opts = []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(2))

		})

		It("When Autoupdate and Publish version annotation are specified in application", func() {
			By("Create configmap-component with 1.4.5 version")
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create application using configmap-component@1.4.5")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1.4.5")
			fmt.Println(app.ObjectMeta.Annotations)
			app.ObjectMeta.Annotations[oam.AnnotationPublishVersion] = "alpha"
			err := k8sClient.Create(ctx, app)
			Expect(err).ShouldNot(BeNil())
			Expect(err.Error()).Should(ContainSubstring("Application has both autoupdate and publishversion annotation. Only one should be present."))

		})
	})

	Context("Disabled", func() {
		It("When specified component version is available", func() {
			By("Create configmap-component with 1.0.0 version")
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create configmap-component with 1.2.0 version")
			updatedComponent := new(v1beta1.ComponentDefinition)
			updatedComponentVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: componentType, Namespace: namespace}, updatedComponent)
				if err != nil {
					return err
				}
				updatedComponent.Spec.Version = updatedComponentVersion
				updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(componentVersion)
				return k8sClient.Update(ctx, updatedComponent)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using configmap-component@1.0.0 version")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
			cm := new(corev1.ConfigMap)
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)
				if err != nil {
					return err
				}
				return nil
			}, 15*time.Second, time.Second).Should(BeNil())
			Expect(cm.Data["expectedVersion"]).To(BeEquivalentTo(componentVersion))
		})

		It("When specified component version is unavailable", func() {
			By("Create configmap-component with 1.2.0 version")
			componentVersion := "1.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			By("Create application using configmap-component@1 version")
			app := updateAppComponent(appTemplate, "app1", namespace, componentType, "first-component", "1")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).ShouldNot(Succeed())
			cm := new(corev1.ConfigMap)
			time.Sleep(sleepTime)
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "comptest", Namespace: namespace}, cm)).ShouldNot(BeNil())

			configmaps := new(corev1.ConfigMapList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			Expect(k8sClient.List(ctx, configmaps, opts...)).To(BeNil())
			Expect(len(configmaps.Items)).To(BeEquivalentTo(0))
		})

		It("When specified trait version is available", func() {
			By("Create scaler-trait with 1.0.0 version and 1 replica")
			traitVersion := "1.0.0"
			traitType := "scaler-trait"
			trait := createTrait(traitVersion, namespace, traitType, "1")
			Expect(k8sClient.Create(ctx, trait)).Should(Succeed())

			By("Create scaler-trait with 1.2.0 version and 2 replica")
			updatedTrait := new(v1beta1.TraitDefinition)
			updatedTraitVersion := "1.2.0"
			Eventually(func() error {
				err := k8sClient.Get(ctx, client.ObjectKey{Name: traitType, Namespace: namespace}, updatedTrait)
				if err != nil {
					return err
				}
				updatedTrait.Spec.Version = updatedTraitVersion
				updatedTrait.Spec.Schematic.CUE.Template = createScalerTraitOutput("2")
				return k8sClient.Update(ctx, updatedTrait)
			}, 15*time.Second, time.Second).Should(BeNil())

			By("Create application using scaler-trait@1.0.0 version")
			app := updatedAppTrait(traitApp, "app1", namespace, traitType, traitVersion)
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())

			By("Wait for application to be created")
			time.Sleep(sleepTime)
			pods := new(corev1.PodList)
			opts := []client.ListOption{
				client.InNamespace(namespace),
				client.MatchingLabels{
					oam.LabelAppName: "app1",
				},
			}
			time.Sleep(5 * time.Second)
			Expect(k8sClient.List(ctx, pods, opts...)).To(BeNil())
			Expect(len(pods.Items)).To(BeEquivalentTo(1))

		})

	})

})

func updateAppComponent(appTemplate v1beta1.Application, appName, namespace, typeName, componentName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)

	app.Spec.Components[0].Name = componentName
	return app
}

func updatedAppTrait(traitApp v1beta1.Application, appName, namespace, typeName, traitVersion string) *v1beta1.Application {
	app := traitApp.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Traits[0].Type = fmt.Sprintf("%s@v%s", typeName, traitVersion)

	return app
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
		Kind:       "ComponentDefinition",
		APIVersion: "core.oam.dev/v1beta1",
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "scaler-trait",
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
