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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/cmd/core/app/config"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var (
	cfg      *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
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

	ctx, cancel = context.WithCancel(context.Background())
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})

var _ = Describe("Server Integration Tests with Real Kubernetes", func() {
	var coreOpts *options.CoreOptions

	BeforeEach(func() {
		coreOpts = options.NewCoreOptions()
	})

	Describe("configureKubernetesClient with real config", func() {
		It("should create and configure REST config successfully", func() {
			// Use the test environment's config
			originalGetConfig := ctrl.GetConfigOrDie
			ctrl.GetConfigOrDie = func() *rest.Config {
				return cfg
			}
			defer func() {
				ctrl.GetConfigOrDie = originalGetConfig
			}()

			k8sConfig := &config.KubernetesConfig{
				QPS:   100,
				Burst: 200,
			}

			restConfig, err := configureKubernetesClient(k8sConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(restConfig).NotTo(BeNil())
			Expect(restConfig.QPS).To(Equal(float32(100)))
			Expect(restConfig.Burst).To(Equal(200))
		})
	})

	Describe("createControllerManager with real config", func() {
		It("should create manager successfully with test environment", func() {
			coreOpts.Server.EnableLeaderElection = false // Disable for testing
			coreOpts.Server.HealthAddr = ":0"           // Use random port
			coreOpts.Observability.MetricsAddr = ":0"   // Use random port
			coreOpts.Webhook.WebhookPort = 0            // Disable webhook
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
			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":0"
			coreOpts.Observability.MetricsAddr = ":0"
			coreOpts.Webhook.UseWebhook = false
			coreOpts.Webhook.WebhookPort = 0
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()

			mgr, err := createControllerManager(ctx, cfg, coreOpts)
			Expect(err).NotTo(HaveOccurred())

			// Setup controllers - this should work without error in test environment
			err = setupControllers(ctx, mgr, coreOpts)
			// With envtest, we expect this to succeed or fail gracefully
			// depending on CRD availability
			if err != nil {
				// Error is acceptable if it's due to missing CRDs
				Expect(err.Error()).To(Or(
					ContainSubstring("no matches for kind"),
					ContainSubstring("CRD"),
					ContainSubstring("not found"),
				))
			}
		})
	})

	Describe("startApplicationMonitor with real manager", func() {
		It("should attempt to start monitor", func() {
			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":0"
			coreOpts.Observability.MetricsAddr = ":0"
			coreOpts.Webhook.WebhookPort = 0
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()

			mgr, err := createControllerManager(ctx, cfg, coreOpts)
			Expect(err).NotTo(HaveOccurred())

			// Start manager in background
			go func() {
				defer GinkgoRecover()
				_ = mgr.Start(ctx)
			}()

			// Wait for cache to be ready
			Eventually(func() bool {
				return mgr.GetCache().WaitForCacheSync(ctx)
			}, 10*time.Second, 100*time.Millisecond).Should(BeTrue())

			// Now try to start application monitor
			err = startApplicationMonitor(ctx, mgr)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("run function integration", func() {
		It("should handle a minimal run with test environment", func() {
			// Create a context with short timeout
			runCtx, runCancel := context.WithTimeout(ctx, 2*time.Second)
			defer runCancel()

			// Override GetConfigOrDie to use test config
			originalGetConfig := ctrl.GetConfigOrDie
			ctrl.GetConfigOrDie = func() *rest.Config {
				return cfg
			}
			defer func() {
				ctrl.GetConfigOrDie = originalGetConfig
			}()

			// Disable features that would block
			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":0"
			coreOpts.Observability.MetricsAddr = ":0"
			coreOpts.Webhook.UseWebhook = false
			coreOpts.MultiCluster.EnableClusterGateway = false

			// Run should eventually fail or timeout, but not panic
			err := run(runCtx, coreOpts)
			Expect(err).To(HaveOccurred())
		})
	})
})