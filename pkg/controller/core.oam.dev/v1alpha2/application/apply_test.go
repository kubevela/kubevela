package application

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha2"
)

var _ = Describe("Test Application apply", func() {
	var handler appHandler
	app := &v1alpha2.Application{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Application",
			APIVersion: "core.oam.dev/v1alpha2",
		},
	}
	var appConfig *v1alpha2.ApplicationConfiguration

	BeforeEach(func() {
		app.Namespace = "apply-test"
		handler = appHandler{
			r:   reconciler,
			app: app,
			l:   reconciler.Log.WithValues("application", "unit-test"),
		}

		appConfig = &v1alpha2.ApplicationConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-app",
				Namespace: "test-ns",
			},
			Spec: v1alpha2.ApplicationConfigurationSpec{
				Components: []v1alpha2.ApplicationConfigurationComponent{
					{

					},
				},
			},
		}
	})

	AfterEach(func() {
	})

	It("test create new application configuration revision", func() {
		app.Name = "test-revision"
		ctx := context.TODO()
		handler.apply(ctx, )
	})
})
