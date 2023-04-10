/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package appfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	coreoam "github.com/oam-dev/kubevela/apis/core.oam.dev"
	corev1beta1 "github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	// +kubebuilder:scaffold:imports
)

var cfg *rest.Config
var scheme *runtime.Scheme
var k8sClient client.Client
var testEnv *envtest.Environment
var definitionDir string
var wd corev1beta1.WorkloadDefinition
var addonNamespace = "test-addon"

func TestAppFile(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cli Suite")
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	ctx := context.Background()
	By("bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "charts", "vela-core", "crds")},
		UseExistingCluster:       &useExistCluster,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())
	scheme = runtime.NewScheme()
	Expect(coreoam.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(clientgoscheme.AddToScheme(scheme)).NotTo(HaveOccurred())
	Expect(crdv1.AddToScheme(scheme)).NotTo(HaveOccurred())
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	definitionDir, err = system.GetCapabilityDir()
	Expect(err).Should(BeNil())
	Expect(os.MkdirAll(definitionDir, 0755)).Should(BeNil())

	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}})).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	workloadData, err := os.ReadFile("testdata/workloadDef.yaml")
	Expect(err).Should(BeNil())

	Expect(yaml.Unmarshal(workloadData, &wd)).Should(BeNil())

	wd.Namespace = addonNamespace
	logf.Log.Info("Creating workload definition", "data", wd)
	Expect(k8sClient.Create(ctx, &wd)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	def, err := os.ReadFile("testdata/terraform-aliyun-oss-workloadDefinition.yaml")
	Expect(err).Should(BeNil())
	var terraformDefinition corev1beta1.WorkloadDefinition
	Expect(yaml.Unmarshal(def, &terraformDefinition)).Should(BeNil())
	terraformDefinition.Namespace = addonNamespace
	logf.Log.Info("Creating workload definition", "data", terraformDefinition)
	Expect(k8sClient.Create(ctx, &terraformDefinition)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))

	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	_ = k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: addonNamespace}})
	_ = k8sClient.Delete(context.Background(), &wd)
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
