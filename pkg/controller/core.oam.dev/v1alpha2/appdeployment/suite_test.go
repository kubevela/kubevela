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

package appdeployment

import (
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.
var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var reconciler *Reconciler
var stop = make(chan struct{})
var ctlManager ctrl.Manager

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	rand.Seed(time.Now().UnixNano())

	By("bootstrapping test environment")
	logf.Log.Info("start application suit test")

	testEnv = &envtest.Environment{
		UseExistingCluster: pointer.BoolPtr(false),
		CRDDirectoryPaths: []string{
			filepath.Join("../../../../..", "charts", "vela-core", "crds"),
			"testdata/istio",
		},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	// +kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
	dm, err := discoverymapper.New(cfg)
	Expect(err).To(BeNil())
	reconciler = NewReconciler(k8sClient, common.Scheme, dm)
	// setup the controller manager since we need the component handler to run in the background
	ctlManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  common.Scheme,
		MetricsBindAddress:      ":8080",
		LeaderElection:          false,
		LeaderElectionNamespace: "default",
		LeaderElectionID:        "test",
	})
	Expect(err).NotTo(HaveOccurred())

	// definitonNs := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
	// Expect(k8sClient.Create(context.Background(), definitonNs.DeepCopy())).Should(BeNil())

	// start the controller in the background so that new componentRevisions are created
	go func() {
		err = ctlManager.Start(stop)
		Expect(err).NotTo(HaveOccurred())
	}()
	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
	close(stop)
})
