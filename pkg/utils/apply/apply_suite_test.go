package apply

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"
	oamstd "github.com/oam-dev/kubevela/apis/standard.oam.dev/v1alpha1"

	"github.com/crossplane/crossplane-runtime/pkg/logging"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var testEnv *envtest.Environment
var cfg *rest.Config
var rawClient client.Client
var k8sApplicator Applicator
var testScheme = runtime.NewScheme()
var ns = "test-apply"
var applyNS corev1.Namespace

func TestApplicator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Applicator Suite")
}

var _ = BeforeSuite(func(done Done) {
	By("Bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ShouldNot(BeNil())

	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	Expect(clientgoscheme.AddToScheme(testScheme)).Should(Succeed())
	Expect(oamcore.AddToScheme(testScheme)).Should(Succeed())
	Expect(oamstd.AddToScheme(testScheme)).Should(Succeed())

	By("Setting up applicator")
	rawClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(rawClient).ShouldNot(BeNil())
	k8sApplicator = NewAPIApplicator(rawClient, logging.NewNopLogger())

	By("Create test namespace")
	applyNS = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	Expect(rawClient.Create(context.Background(), &applyNS)).Should(Succeed())

	close(done)
}, 300)

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).Should(Succeed())
})
