package applicationconfiguration

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/crossplane/oam-kubernetes-runtime/apis/core"
)

var scheme = runtime.NewScheme()
var crd crdv1.CustomResourceDefinition
var reqResource metav1.GroupVersionResource
var decoder *admission.Decoder

func TestApplicationConfigurationWebHandler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ApplicationConfiguration Web handler")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
		o.DestWritter = os.Stdout
	}))
	By("Setup scheme")
	err := core.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	err = clientgoscheme.AddToScheme(scheme)
	Expect(err).Should(BeNil())
	// the crd we will refer to
	crd = crdv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo.example.com",
		},
		Spec: crdv1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: crdv1.CustomResourceDefinitionNames{
				Kind:     "Foo",
				ListKind: "FooList",
				Plural:   "foo",
				Singular: "foo",
			},
			Versions: []crdv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]crdv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]crdv1.JSONSchemaProps{
										"key": {Type: "string"},
									},
								},
								"status": {
									Type: "object",
									Properties: map[string]crdv1.JSONSchemaProps{
										"key": {Type: "string"},
									},
								},
							},
						},
					},
				},
			},
			Scope: crdv1.NamespaceScoped,
		},
	}
	By("Prepare for the admission resource")
	reqResource = metav1.GroupVersionResource{Group: "core.oam.dev", Version: "v1alpha2", Resource: "applicationconfigurations"}
	By("Prepare for the admission decoder")
	decoder, err = admission.NewDecoder(scheme)
	Expect(err).Should(BeNil())
	By("Finished test bootstrap")
	close(done)
})
