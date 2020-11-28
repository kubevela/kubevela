package discoverymapper

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var scheme = runtime.NewScheme()

func TestMapper(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Test Mapper Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(crdv1.AddToScheme(scheme)).Should(BeNil())
	// +kubebuilder:scaffold:scheme
	By("Create the k8s client")
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("Tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Mapper discovery resources", func() {

	It("discovery built-in k8s resource", func() {
		dism, err := New(cfg)
		Expect(err).Should(BeNil())
		mapper, err := dism.GetMapper()
		Expect(err).Should(BeNil())
		mapping, err := mapper.RESTMapping(schema.GroupKind{Group: "apps", Kind: "Deployment"}, "v1")
		Expect(err).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		}))
	})

	It("discovery CRD", func() {

		By("Check built-in resource")
		dism, err := New(cfg)
		Expect(err).Should(BeNil())
		mapper, err := dism.GetMapper()
		Expect(err).Should(BeNil())
		var mapping *meta.RESTMapping
		mapping, err = mapper.RESTMapping(schema.GroupKind{Group: "", Kind: "Pod"}, "v1")
		Expect(err).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		}))

		By("CRD should be discovered after refresh")
		crd := crdv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "foos.example.com",
				Labels: map[string]string{"crd": "dependency"},
			},
			Spec: crdv1.CustomResourceDefinitionSpec{
				Group: "example.com",
				Names: crdv1.CustomResourceDefinitionNames{
					Kind:   "Foo",
					Plural: "foos",
				},
				Versions: []crdv1.CustomResourceDefinitionVersion{{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}, {
					Name:   "v1beta1",
					Served: true,
					Schema: &crdv1.CustomResourceValidation{
						OpenAPIV3Schema: &crdv1.JSONSchemaProps{
							Type: "object",
						}},
				}},
				Scope: crdv1.NamespaceScoped,
			},
		}
		Expect(k8sClient.Create(context.Background(), &crd)).Should(BeNil())
		updatedCrdObj := crdv1.CustomResourceDefinition{}
		Eventually(func() bool {
			if err := k8sClient.Get(context.Background(),
				client.ObjectKey{Name: "foos.example.com"}, &updatedCrdObj); err != nil {
				return false
			}
			return len(updatedCrdObj.Spec.Versions) == 2
		}, 3*time.Second, time.Second).Should(BeTrue())

		Eventually(func() error {
			mapping, err = dism.RESTMapping(schema.GroupKind{Group: "example.com", Kind: "Foo"}, "v1")
			return err
		}, time.Second*2, time.Millisecond*300).Should(BeNil())
		Expect(mapping.Resource).Should(Equal(schema.GroupVersionResource{
			Group:    "example.com",
			Version:  "v1",
			Resource: "foos",
		}))

		var kinds []schema.GroupVersionKind
		Eventually(func() error {
			kinds, err = dism.KindsFor(schema.GroupVersionResource{Group: "example.com", Version: "", Resource: "foos"})
			return err
		}, time.Second*10, time.Millisecond*300).Should(BeNil())
		Expect(kinds).Should(Equal([]schema.GroupVersionKind{
			{Group: "example.com", Version: "v1", Kind: "Foo"},
			{Group: "example.com", Version: "v1beta1", Kind: "Foo"},
		}))
		kinds, err = dism.KindsFor(schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "foos"})
		Expect(err).Should(BeNil())
		Expect(kinds).Should(Equal([]schema.GroupVersionKind{{Group: "example.com", Version: "v1", Kind: "Foo"}}))
	})
})
