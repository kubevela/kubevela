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

package dryrun

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	coreoam "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/appfile"
	"github.com/oam-dev/kubevela/pkg/cue/packages"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	oamutil "github.com/oam-dev/kubevela/pkg/oam/util"
)

var cfg *rest.Config
var scheme *runtime.Scheme
var k8sClient client.Client
var testEnv *envtest.Environment
var dm discoverymapper.DiscoveryMapper
var pd *packages.PackageDiscover
var dryrunOpt *Option
var diffOpt *LiveDiffOption

func TestDryRun(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t,
		"Cli Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	By("bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "..", "charts", "vela-core", "crds")},
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
	dm, err = discoverymapper.New(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(dm).ToNot(BeNil())
	pd, err = packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(pd).ToNot(BeNil())

	By("Prepare capability definitions")
	myingressYAML := readDataFromFile("./testdata/td-myingress.yaml")
	myscalerYAML := readDataFromFile("./testdata/td-myscaler.yaml")
	myworkerYAML := readDataFromFile("./testdata/cd-myworker.yaml")

	myworkerDef, err := oamutil.UnMarshalStringToComponentDefinition(myworkerYAML)
	Expect(err).Should(BeNil())
	myingressDef, err := oamutil.UnMarshalStringToTraitDefinition(myingressYAML)
	Expect(err).Should(BeNil())
	myscalerDef, err := oamutil.UnMarshalStringToTraitDefinition(myscalerYAML)
	Expect(err).Should(BeNil())

	cdMyWorker, err := oamutil.Object2Unstructured(myworkerDef)
	Expect(err).Should(BeNil())
	tdMyIngress, err := oamutil.Object2Unstructured(myingressDef)
	Expect(err).Should(BeNil())
	tdMyScaler, err := oamutil.Object2Unstructured(myscalerDef)
	Expect(err).Should(BeNil())

	dryrunOpt = NewDryRunOption(k8sClient, cfg, dm, pd, []oam.Object{cdMyWorker, tdMyIngress, tdMyScaler})
	diffOpt = &LiveDiffOption{DryRun: dryrunOpt, Parser: appfile.NewApplicationParser(k8sClient, dm, pd)}

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

func readDataFromFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}
