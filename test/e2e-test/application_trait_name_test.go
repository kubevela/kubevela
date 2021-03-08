package controllers_test

import (
	"context"
	"fmt"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Validate Component of Application", func() {
	var app v1alpha2.Application
	var comp1 v1alpha2.ApplicationComponent
	var components []v1alpha2.ApplicationComponent
	ctx := context.Background()
	BeforeEach(func() {
		logf.Log.Info("Start to run a test, validate component of application trait name unique")
		comp1 = v1alpha2.ApplicationComponent{
			Name:         "myservice1",
			WorkloadType: "worker",
			Settings:     runtime.RawExtension{Raw: []byte(`{"cmd":["sleep","1000"],"image":"busybox","config":"myconfig"}`)},
			Traits: append(make([]v1alpha2.ApplicationTrait, 0), v1alpha2.ApplicationTrait{
				Name: "webservice",
				Properties: runtime.RawExtension{
					Raw: []byte(`{"domain": "example.com", "http":{"/": 8080}}`),
				},
			}),
		}
		components = append(make([]v1alpha2.ApplicationComponent, 0), comp1)
		app = v1alpha2.Application{
			ObjectMeta: metav1.ObjectMeta{Name: "myapp", Namespace: "default"},
			Spec: v1alpha2.ApplicationSpec{
				Components: components,
			},
		}
	})
	When("Trait Name unique of Component", func() {
		It("Apply App", func() {
			By("Apply App Success")
			Expect(k8sClient.Create(ctx, app.DeepCopyObject())).Should(Succeed())
			Expect(k8sClient.Delete(ctx, &app)).Should(Succeed())
			app.Spec.Components[0].Traits = append(app.Spec.Components[0].Traits, v1alpha2.ApplicationTrait{
				Name:       "webservice",
				Properties: runtime.RawExtension{Raw: []byte(`{"domain": "example.com", "http":{"/": 8080}}`)},
			})
			By("Apply App Failed")
			Expect(k8sClient.Create(ctx, app.DeepCopyObject())).Should(ContainSubstring(fmt.Sprintf("traits name (%q) conflict between of component %q", "webservice", comp1.Name)))
		})
	})

	AfterEach(func() {
		logf.Log.Info("Cleanup application")
		Expect(k8sClient.Delete(ctx, &app)).Should(Succeed())
	})
})
