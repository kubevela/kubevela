/*
Copyright 2026 The KubeVela Authors.

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

// Package addonmoduletest is a self-contained envtest suite proving the
// outer Application -> "type: addon" component -> nested addon-<name>
// Application -> "type: module" component -> nested module-<name>
// Application chain actually reconciles end to end.
//
// It cannot use the live-cluster harness in test/e2e-addon-test (which needs
// a real vela-core deployment reached via config.GetConfigOrDie, a built vela
// CLI, and a live addon/module registry -- none of which this sandbox has).
// Instead it boots its own envtest.Environment and runs the real Application
// controller (the same controller cmd/core/app/server.go wires up in
// production) against it, so Application objects created here are actually
// reconciled by real, unmodified production code -- not a hand-rolled test
// harness. The only substitution is the addon/module render seam
// (pkg/addon/service/api and pkg/module/service/api's injectable Renderer),
// which a real cluster fills from a registry; here it is filled by
// fixtureAddonRenderer/fixtureModuleRenderer in addon_module_nest_test.go so
// the test needs no network access or mock registry server.
package addonmoduletest

import (
	"context"
	"path/filepath"
	sysruntime "runtime"
	"strings"
	"testing"
	"time"

	cuexv1alpha1 "github.com/kubevela/pkg/apis/cue/v1alpha1"
	"github.com/kubevela/pkg/util/singleton"
	testdef "github.com/kubevela/pkg/util/test/definition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	core "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	appcontroller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev/v1beta1/application"
	"github.com/oam-dev/kubevela/pkg/features"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	// +kubebuilder:scaffold:imports
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    = runtime.NewScheme()
	cancelMgr context.CancelFunc
)

// systemNamespace is where the "addon" and "module" ComponentDefinitions and
// every nested addon-<name>/module-<name> Application land, matching the
// real "vela addon enable" convention.
const systemNamespace = "vela-system"

func TestAddonModuleNest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Addon/Module Nest Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	By("enabling the addon/module component feature gates")
	Expect(utilfeature.DefaultMutableFeatureGate.Set(string(features.EnableAddonComponent) + "=true")).To(Succeed())
	Expect(utilfeature.DefaultMutableFeatureGate.Set(string(features.EnableModuleComponent) + "=true")).To(Succeed())

	By("bootstrapping the envtest control plane")
	_, thisFile, _, _ := sysruntime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
		CRDDirectoryPaths:        []string{filepath.Join(repoRoot, "charts", "vela-core", "crds")},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(v1beta1.SchemeBuilder.AddToScheme(scheme)).To(Succeed())
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(crdv1.AddToScheme(scheme)).To(Succeed())
	Expect(cuexv1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// The workflow engine and CueX providers read the cluster client and
	// rest config from these process-wide singletons rather than from the
	// Reconciler's own client, so without this the controller's workflow
	// step falls back to the ambient (host) kubeconfig instead of envtest's.
	singleton.KubeClient.Set(k8sClient)
	singleton.KubeConfig.Set(cfg)
	singleton.DynamicClient.Set(dynamicfake.NewSimpleDynamicClient(scheme))

	ctx := context.Background()
	By("creating the vela-system namespace")
	Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: systemNamespace}})).To(Succeed())

	By("installing the addon, module and k8s-objects ComponentDefinitions")
	replaceSystemNS := func(s string) string {
		return strings.ReplaceAll(s, `{{ include "systemDefinitionNamespace" . }}`, systemNamespace)
	}
	for _, name := range []string{"addon", "module", "k8s-objects"} {
		defPath := filepath.Join(repoRoot, "charts", "vela-core", "templates", "defwithtemplate", name+".yaml")
		Expect(testdef.InstallDefinitionFromYAML(ctx, k8sClient, defPath, replaceSystemNS)).To(Succeed())
	}

	By("starting the real Application controller (the same one cmd/core/app/server.go wires up)")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		LeaderElection:          false,
		LeaderElectionNamespace: "default",
		LeaderElectionID:        "addon-module-nest-test",
	})
	Expect(err).NotTo(HaveOccurred())
	Expect(appcontroller.Setup(mgr, core.Args{AppRevisionLimit: 5, ConcurrentReconciles: 1})).To(Succeed())

	var mgrCtx context.Context
	mgrCtx, cancelMgr = context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(mgrCtx)).To(Succeed())
	}()
	multicluster.InitClusterInfo(cfg)
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if cancelMgr != nil {
		cancelMgr()
	}
	Expect(testEnv.Stop()).To(Succeed())
})
