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

var _ = Describe("KubeVela Application Auto update", Ordered, func() {
	ctx := context.Background()
	var namespace string
	var ns corev1.Namespace
	var sleepTime = 90 * time.Second

	BeforeEach(func() {
		namespace = randomNamespaceName("app-autoupdate-e2e-test")
		ns = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		ctx.Value(1)
		Eventually(func() error {
			return k8sClient.Create(ctx, &ns)
		}, time.Second*3, time.Microsecond*300).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	})

	FIt("test1", func() {
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
			updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion, componentVersion)
			return k8sClient.Update(ctx, updatedComponent)
		}, 15*time.Second, time.Second).Should(BeNil())

		app := createApp("app1", namespace, componentType, "first-component", componentVersion)
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
			updatedComponent.Spec.Schematic.CUE.Template = createOutputConfigMap(updatedComponentVersion, componentVersion)
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
	AfterEach(func() {
		k8sClient.DeleteAllOf(ctx, &v1beta1.ComponentDefinition{}, client.InNamespace(namespace))
		k8sClient.DeleteAllOf(ctx, &v1beta1.Application{}, client.InNamespace(namespace))
		Expect(k8sClient.Delete(ctx, &ns)).Should(BeNil())
	})

})

func createApp(appName, namespace, typeName, componentName, componentVersion string) *v1beta1.Application {
	app := appTemplate.DeepCopy()
	app.ObjectMeta.Name = appName
	app.SetNamespace(namespace)
	fmt.Println(app.GetNamespace())
	fmt.Println(fmt.Sprintf("%s@v%s", typeName, componentVersion))
	app.Spec.Components[0].Type = fmt.Sprintf("%s@v%s", typeName, componentVersion)

	app.Spec.Components[0].Name = componentName
	return app
}

func createComponent(componentVersion, namespace, name string) *v1beta1.ComponentDefinition {
	component := configMapComponent.DeepCopy()
	component.ObjectMeta.Name = name
	component.Spec.Version = componentVersion
	component.Spec.Schematic.CUE.Template = createOutputConfigMap("1.0.0", componentVersion)
	component.SetNamespace(namespace)
	return component
}

func createOutputConfigMap(fromVersion, toVersion string) string {
	return strings.Replace(configMapTemplate, fromVersion, toVersion, 1)
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

var configMapTemplate = `output: {
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
