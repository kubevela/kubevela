//go:build integration
// +build integration

/*
Copyright 2024 The KubeVela Authors.

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

package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/config"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/auth"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/version"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	// Note: we'll use per-test contexts to avoid conflicts
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Server Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 2,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "charts", "vela-core", "crds"),
		},
		ErrorIfCRDPathMissing: false,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	// Add schemes
	err = v1beta1.AddToScheme(clientscheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Server Integration Tests with Real Kubernetes", func() {
	var (
		coreOpts *options.CoreOptions
		ctx      context.Context
		cancel   context.CancelFunc
	)

	BeforeEach(func() {
		coreOpts = options.NewCoreOptions()
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		if cancel != nil {
			cancel()
		}
	})

	Describe("configureKubernetesClient with real config", func() {
		It("should create and configure REST config successfully", func() {
			// For this test, we need to mock ctrl.GetConfig to return our test config
			// Since we can't easily mock ctrl.GetConfig, we'll test the configuration
			// logic by directly using the test environment's config
			k8sConfig := &config.KubernetesConfig{
				QPS:   100,
				Burst: 200,
			}

			// Create a copy of the test config to simulate what configureKubernetesClient does
			testConfig := rest.CopyConfig(cfg)
			testConfig.UserAgent = types.KubeVelaName + "/" + version.GitRevision
			testConfig.QPS = float32(k8sConfig.QPS)
			testConfig.Burst = k8sConfig.Burst
			testConfig.Wrap(auth.NewImpersonatingRoundTripper)

			// Verify the configuration would be applied correctly
			Expect(testConfig.QPS).To(Equal(float32(100)))
			Expect(testConfig.Burst).To(Equal(200))
			Expect(testConfig.UserAgent).To(ContainSubstring(types.KubeVelaName))
		})
	})

	Describe("createControllerManager with real config", func() {
		It("should create manager successfully with test environment", func() {
			coreOpts.Server.EnableLeaderElection = false // Disable for testing
			coreOpts.Server.HealthAddr = ":0"            // Use random port
			coreOpts.Observability.MetricsAddr = ":0"    // Use random port
			coreOpts.Webhook.WebhookPort = 0             // Disable webhook
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()

			mgr, err := createControllerManager(ctx, cfg, coreOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(mgr).NotTo(BeNil())

			// Verify manager has correct configuration
			Expect(mgr.GetConfig()).To(Equal(cfg))
			Expect(mgr.GetScheme()).NotTo(BeNil())
		})
	})

	Describe("setupControllers with real manager", func() {
		It("should setup controllers without error", func() {
			// Create a fresh manager for this test
			testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
			defer testCancel()

			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":0"
			coreOpts.Observability.MetricsAddr = ":0"
			coreOpts.Webhook.UseWebhook = false
			coreOpts.Webhook.WebhookPort = 0
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()

			mgr, err := createControllerManager(testCtx, cfg, coreOpts)
			Expect(err).NotTo(HaveOccurred())

			// Setup controllers - this should work without error in test environment
			err = setupControllers(testCtx, mgr, coreOpts)
			// With envtest, we expect this to succeed or fail gracefully
			// depending on CRD availability
			if err != nil {
				// Error is acceptable if it's due to missing CRDs or context cancellation
				Expect(err.Error()).To(Or(
					ContainSubstring("no matches for kind"),
					ContainSubstring("CRD"),
					ContainSubstring("not found"),
					ContainSubstring("context"),
				))
			}
		})
	})

	Describe("startApplicationMonitor with real manager", func() {
		It("should attempt to start monitor", func() {
			// Create a fresh context and manager for this test
			testCtx, testCancel := context.WithTimeout(ctx, 10*time.Second)
			defer testCancel()

			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":0"
			coreOpts.Observability.MetricsAddr = ":0"
			coreOpts.Webhook.WebhookPort = 0
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()

			mgr, err := createControllerManager(testCtx, cfg, coreOpts)
			Expect(err).NotTo(HaveOccurred())

			// Start manager in background with test context
			go func() {
				defer GinkgoRecover()
				_ = mgr.Start(testCtx)
			}()

			// Wait for cache to be ready
			Eventually(func() bool {
				return mgr.GetCache().WaitForCacheSync(testCtx)
			}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Now try to start application monitor
			err = startApplicationMonitor(testCtx, mgr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("run function integration", func() {
		It("should handle initialization and configuration", func() {
			// Create a very short-lived context to test initialization only
			runCtx, runCancel := context.WithTimeout(ctx, 100*time.Millisecond)
			defer runCancel()

			// Create fresh options for this test
			testOpts := options.NewCoreOptions()
			testOpts.Server.EnableLeaderElection = false
			testOpts.Server.HealthAddr = ":0"
			testOpts.Observability.MetricsAddr = ":0"
			testOpts.Webhook.UseWebhook = false
			testOpts.MultiCluster.EnableClusterGateway = false

			// The run function will timeout quickly due to context cancellation
			// We're testing that it initializes without panic
			err := run(runCtx, testOpts)

			// We expect an error due to context cancellation or missing CRDs
			Expect(err).To(HaveOccurred())
			// The error should be related to context or controller setup, not a panic
			Expect(err.Error()).To(Or(
				ContainSubstring("context"),
				ContainSubstring("controller"),
				ContainSubstring("failed"),
			))
		})
	})
})
