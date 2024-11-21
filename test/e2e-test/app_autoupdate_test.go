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
		namespace = randomNamespaceName("app-autoupdate-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	AfterEach(func() {
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.DefinitionRevision{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

	Context("Enabled", func() {
		It("When specified exact component version available", func() {
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

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

			app := createApp(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
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
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			app := createApp(appTemplate, "app1", namespace, componentType, "first-component", "1.4")
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

		XIt("When specified component version is unavailable, and after app creation new version of component is available", func() {
			componentVersion := "2.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

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
			app := createApp(appTemplate, "app1", namespace, componentType, "first-component", "2")
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
			componentVersion := "1.4.5"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			app := createApp(appWithTwoComponentTemplate, "app1", namespace, componentType, "first-component", "1.4")

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

	})

	Context("Disabled", func() {
		It("When specified component version is available", func() {
			componentVersion := "1.0.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

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

			app := createApp(appTemplate, "app1", namespace, componentType, "first-component", componentVersion)
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
			componentVersion := "1.2.0"
			componentType := "configmap-component"
			component := createComponent(componentVersion, namespace, componentType)
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			app := createApp(appTemplate, "app1", namespace, componentType, "first-component", "1")
			app.ObjectMeta.Annotations[oam.AnnotationAutoUpdate] = "false"
			Expect(k8sClient.Create(ctx, app)).Should(Succeed())
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
	})

})

func createApp(appTemplate v1beta1.Application, appName, namespace, typeName, componentName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	app.Spec.Components[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)

	app.Spec.Components[0].Name = componentName
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

var scalerTraitOutputTemplate = `output: {
	patch: spec: replicas: 1
}`

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

var traitApp = &v1beta1.Application{
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
				Name: "webservice-component",
				Type: "webservice",
				// Properties: {},
			},
		},
	},
}
