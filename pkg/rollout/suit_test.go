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

package rollout

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/discovery"
	ocmclusterv1 "open-cluster-management.io/api/cluster/v1"
	ocmclusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ocmworkv1 "open-cluster-management.io/api/work/v1"

	v12 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	kruisev1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"

	"github.com/kubevela/workflow/pkg/cue/packages"

	coreoam "github.com/oam-dev/kubevela/apis/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	// +kubebuilder:scaffold:imports
)

var cfg *rest.Config
var scheme *runtime.Scheme
var k8sClient client.Client
var testEnv *envtest.Environment
var dm discoverymapper.DiscoveryMapper
var pd *packages.PackageDiscover
var testns string
var dc *discovery.DiscoveryClient

func TestAddon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kruise rollout Suite test")
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	By("bootstrapping test environment")
	useExistCluster := false
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "charts", "vela-core", "crds"), filepath.Join("", "testdata")},
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
	_ = ocmclusterv1alpha1.Install(scheme)
	_ = ocmclusterv1.Install(scheme)
	_ = ocmworkv1.Install(scheme)
	_ = kruisev1alpha1.AddToScheme(scheme)
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())

	dc, err = discovery.NewDiscoveryClientForConfig(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(dc).ShouldNot(BeNil())

	dm, err = discoverymapper.New(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(dm).ToNot(BeNil())
	pd, err = packages.NewPackageDiscover(cfg)
	Expect(err).ToNot(HaveOccurred())
	Expect(pd).ToNot(BeNil())
	testns = "vela-system"
	Expect(k8sClient.Create(context.Background(),
		&v12.Namespace{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}, ObjectMeta: metav1.ObjectMeta{
			Name: testns,
		}}))

	close(done)
}, 120)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
