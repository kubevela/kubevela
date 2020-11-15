package plugins

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/oam-dev/kubevela/pkg/utils/system"

	corev1alpha2 "github.com/crossplane/oam-kubernetes-runtime/apis/core"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"
	"github.com/crossplane/oam-kubernetes-runtime/pkg/oam/util"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
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
var scheme *runtime.Scheme
var k8sClient client.Client
var testEnv *envtest.Environment
var definitionDir string
var td v1alpha2.TraitDefinition
var wd, websvcWD v1alpha2.WorkloadDefinition

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"CLI Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))
	ctx := context.Background()
	By("bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:  []string{filepath.Join("..", "..", "charts", "vela-core", "crds")},
		UseExistingCluster: &useExistCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	scheme = runtime.NewScheme()
	Expect(corev1alpha2.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(clientgoscheme.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(v1beta1.AddToScheme(scheme)).NotTo(HaveOccurred())
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	definitionDir, err = system.GetCapabilityDir()
	Expect(err).Should(BeNil())
	Expect(os.MkdirAll(definitionDir, 0755)).Should(BeNil())

	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DefinitionNamespace}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	traitdata, err := ioutil.ReadFile("testdata/traitDef.yaml")
	Expect(err).Should(BeNil())
	Expect(yaml.Unmarshal(traitdata, &td)).Should(BeNil())

	td.Namespace = DefinitionNamespace
	logf.Log.Info("Creating trait definition", "data", td)
	Expect(k8sClient.Create(ctx, &td)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	workloaddata, err := ioutil.ReadFile("testdata/workloadDef.yaml")
	Expect(err).Should(BeNil())

	Expect(yaml.Unmarshal(workloaddata, &wd)).Should(BeNil())

	wd.Namespace = DefinitionNamespace
	logf.Log.Info("Creating workload definition", "data", wd)
	Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	websvcWorkloadData, err := ioutil.ReadFile("testdata/websvcWorkloadDef.yaml")
	Expect(err).Should(BeNil())

	Expect(yaml.Unmarshal(websvcWorkloadData, &websvcWD)).Should(BeNil())
	websvcWD.Namespace = DefinitionNamespace
	logf.Log.Info("Creating workload definition whose CUE template from remote", "data", &websvcWD)
	Expect(k8sClient.Create(ctx, &websvcWD)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	close(done)
}, 60)

var DefinitionNamespace = "testdef"
var _ = AfterSuite(func() {
	By("tearing down the test environment")
	_ = k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: DefinitionNamespace}})
	_ = k8sClient.Delete(context.Background(), &td)
	_ = k8sClient.Delete(context.Background(), &wd)
	_ = k8sClient.Delete(context.Background(), &websvcWD)
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
