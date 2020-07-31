package plugins

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"CLI Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	By("bootstrapping test environment")
	useExistCluster := true
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "config", "crd", "bases")},
		UseExistingCluster: &useExistCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	scheme := runtime.NewScheme()
	Expect(corev1alpha2.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(clientgoscheme.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(v1beta1.AddToScheme(scheme)).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	crd := v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "traitdefinitions.core.oam.dev",
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group: "core.oam.dev",
			Names: v1beta1.CustomResourceDefinitionNames{
				Kind:     "TraitDefinition",
				ListKind: "TraitDefinitionList",
				Plural:   "traitdefinitions",
				Singular: "traitdefinition",
			},
			Versions: []v1beta1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	crd = v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workloaddefinitions.core.oam.dev",
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group: "core.oam.dev",
			Names: v1beta1.CustomResourceDefinitionNames{
				Kind:     "WorkloadDefinition",
				ListKind: "WorkloadDefinitionList",
				Plural:   "workloaddefinitions",
				Singular: "workloaddefinition",
			},
			Versions: []v1beta1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha2",
					Served:  true,
					Storage: true,
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), &crd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
	close(done)
}, 60)

var DefinitionNamespace = "testdef"
var _ = AfterSuite(func() {
	By("tearing down the test environment")
	k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DefinitionNamespace}})

	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
