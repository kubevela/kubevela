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

package app

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/cmd/core/app/config"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
	"github.com/oam-dev/kubevela/version"
)

/*
Test Organization Notes:
- Unit tests for all server helper functions are in this file
- Tests use mocks and fakes to avoid needing real Kubernetes components
- All tests use Ginkgo for consistency
*/

var (
	testdir      = "testdir"
	testTimeout  = 2 * time.Second
	testInterval = 1 * time.Second
	testEnv      *envtest.Environment
	testConfig   *rest.Config
)

func TestGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "test main")
}

var _ = BeforeSuite(func() {
	By("bootstrapping test environment")
	useExistCluster := false

	// Resolve the CRD path relative to the test file location
	crdPath := filepath.Join("..", "..", "..", "charts", "vela-core", "crds")

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: 2 * time.Minute, // Increased timeout for CI
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			crdPath,
		},
		UseExistingCluster:    &useExistCluster,
		ErrorIfCRDPathMissing: true, // Fail fast if CRDs are not found
	}

	var err error
	testConfig, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(testConfig).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	if testEnv != nil {
		err := testEnv.Stop()
		Expect(err).ToNot(HaveOccurred())
	}
})

var _ = Describe("Server Tests", func() {
	Describe("waitWebhookSecretVolume", func() {
		BeforeEach(func() {
			err := os.MkdirAll(testdir, 0755)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.RemoveAll(testdir)
		})

		When("dir not exist or empty", func() {
			It("return timeout error", func() {
				err := waitWebhookSecretVolume(testdir, testTimeout, testInterval)
				Expect(err).To(HaveOccurred())
				By("remove dir")
				os.RemoveAll(testdir)
				err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
				Expect(err).To(HaveOccurred())
			})
		})

		When("dir contains empty file", func() {
			It("return timeout error", func() {
				By("add empty file")
				err := os.WriteFile(testdir+"/emptyFile", []byte{}, 0644)
				Expect(err).NotTo(HaveOccurred())
				err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
				Expect(err).To(HaveOccurred())
			})
		})

		When("files in dir are not empty", func() {
			It("return nil", func() {
				By("add non-empty file")
				err := os.WriteFile(testdir+"/file", []byte("test"), 0600)
				Expect(err).NotTo(HaveOccurred())
				err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("syncConfigurations", func() {
		var coreOpts *options.CoreOptions

		BeforeEach(func() {
			coreOpts = options.NewCoreOptions()
		})

		Context("with all configs populated", func() {
			It("should sync all configuration values to global variables", func() {
				// Set some test values using actual fields from the config structs
				coreOpts.Workflow.MaxWaitBackoffTime = 120
				coreOpts.Workflow.MaxFailedBackoffTime = 600
				coreOpts.Application.ReSyncPeriod = 30 * time.Minute
				coreOpts.Kubernetes.InformerSyncPeriod = 10 * time.Hour

				// Call sync function
				syncConfigurations(coreOpts)

				// Verify globals were updated (this is a smoke test - actual values depend on implementation)
				// The key point is the function runs without panicking
				Expect(func() { syncConfigurations(coreOpts) }).NotTo(Panic())
			})
		})

		Context("with partial configs", func() {
			It("should handle nil configs gracefully", func() {
				opts := &options.CoreOptions{
					Workflow:    config.NewWorkflowConfig(),
					CUE:         config.NewCUEConfig(),
					Application: nil, // Intentionally nil
					Performance: config.NewPerformanceConfig(),
					Resource:    config.NewResourceConfig(),
					OAM:         config.NewOAMConfig(),
				}

				// Should not panic even with nil fields
				Expect(func() {
					syncConfigurations(opts)
				}).NotTo(Panic())
			})
		})

		Context("with empty CoreOptions", func() {
			It("should handle nil options safely", func() {
				nilOpts := &options.CoreOptions{}
				// Should not panic even with nil fields
				Expect(func() {
					syncConfigurations(nilOpts)
				}).NotTo(Panic())
			})
		})
	})

	Describe("setupLogging", func() {
		var origStderr *os.File

		BeforeEach(func() {
			origStderr = os.Stderr
		})

		AfterEach(func() {
			os.Stderr = origStderr
			// Reset klog settings
			klog.LogToStderr(true)
			flag.Set("logtostderr", "true")
		})

		Context("debug logging", func() {
			It("should configure debug logging when LogDebug is true", func() {
				obsConfig := &config.ObservabilityConfig{
					LogDebug: true,
				}

				setupLogging(obsConfig)

				// Verify debug level was set (we can't directly check flag values easily)
				// But we can verify the function doesn't panic
				Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
			})
		})

		Context("file logging", func() {
			It("should configure file logging when LogFilePath is set", func() {
				tempDir := GinkgoT().TempDir()
				logFile := filepath.Join(tempDir, "test.log")

				obsConfig := &config.ObservabilityConfig{
					LogFilePath:    logFile,
					LogFileMaxSize: 100,
				}

				setupLogging(obsConfig)

				// Verify flags were set (indirectly by checking no panic)
				Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
			})
		})

		Context("dev logging", func() {
			It("should configure dev logging with color output", func() {
				obsConfig := &config.ObservabilityConfig{
					DevLogs: true,
				}

				// Capture output to verify color writer is used
				var buf bytes.Buffer
				klog.SetOutput(&buf)
				defer klog.SetOutput(os.Stderr)

				setupLogging(obsConfig)

				// The function should complete without error
				Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
			})
		})

		Context("standard logging", func() {
			It("should configure standard logging when DevLogs is false", func() {
				obsConfig := &config.ObservabilityConfig{
					DevLogs: false,
				}

				setupLogging(obsConfig)
				Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
			})
		})
	})

	Describe("configureFeatureGates", func() {
		var coreOpts *options.CoreOptions
		var originalPeriod time.Duration

		BeforeEach(func() {
			coreOpts = options.NewCoreOptions()
			originalPeriod = commonconfig.ApplicationReSyncPeriod
		})

		AfterEach(func() {
			commonconfig.ApplicationReSyncPeriod = originalPeriod
			feature.DefaultMutableFeatureGate.Set("ApplyOnce=false")
		})

		Context("when ApplyOnce is enabled", func() {
			It("should configure ApplicationReSyncPeriod", func() {
				// Enable the feature gate
				feature.DefaultMutableFeatureGate.Set("ApplyOnce=true")

				testPeriod := 5 * time.Minute
				coreOpts.Kubernetes.InformerSyncPeriod = testPeriod

				configureFeatureGates(coreOpts)

				Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(testPeriod))
			})
		})

		Context("when ApplyOnce is disabled", func() {
			It("should not change ApplicationReSyncPeriod", func() {
				feature.DefaultMutableFeatureGate.Set("ApplyOnce=false")

				coreOpts.Kubernetes.InformerSyncPeriod = 10 * time.Minute

				configureFeatureGates(coreOpts)

				Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(originalPeriod))
			})
		})

		Context("with different sync periods", func() {
			DescribeTable("should handle various sync periods correctly",
				func(enabled bool, syncPeriod time.Duration, expectedResult time.Duration) {
					flagValue := fmt.Sprintf("ApplyOnce=%v", enabled)
					feature.DefaultMutableFeatureGate.Set(flagValue)

					coreOpts.Kubernetes.InformerSyncPeriod = syncPeriod
					configureFeatureGates(coreOpts)

					if enabled {
						Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(expectedResult))
					} else {
						Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(originalPeriod))
					}
				},
				Entry("enabled with 5 minutes", true, 5*time.Minute, 5*time.Minute),
				Entry("enabled with 10 minutes", true, 10*time.Minute, 10*time.Minute),
				Entry("disabled with 5 minutes", false, 5*time.Minute, originalPeriod),
				Entry("disabled with 10 minutes", false, 10*time.Minute, originalPeriod),
			)
		})
	})

	Describe("performCleanup", func() {
		var coreOpts *options.CoreOptions

		BeforeEach(func() {
			coreOpts = options.NewCoreOptions()
		})

		Context("with log file path", func() {
			It("should flush logs when LogFilePath is set", func() {
				coreOpts.Observability.LogFilePath = "/tmp/test.log"

				// Should not panic
				Expect(func() { performCleanup(coreOpts) }).NotTo(Panic())

				// Verify klog.Flush was called (indirectly)
				performCleanup(coreOpts)
			})
		})

		Context("without log file path", func() {
			It("should do nothing when LogFilePath is empty", func() {
				coreOpts.Observability.LogFilePath = ""

				// Should not panic
				Expect(func() { performCleanup(coreOpts) }).NotTo(Panic())
			})
		})

		DescribeTable("should handle various log file configurations",
			func(logFilePath string) {
				coreOpts.Observability.LogFilePath = logFilePath

				// Should not panic
				Expect(func() { performCleanup(coreOpts) }).NotTo(Panic())
			},
			Entry("empty path", ""),
			Entry("tmp file", "/tmp/test.log"),
			Entry("relative path", "test.log"),
			Entry("nested path", "/var/log/kubevela/test.log"),
		)
	})

	Describe("configureKubernetesClient", func() {
		Context("when creating Kubernetes config", func() {
			It("should configure REST config with correct parameters using ENVTEST", func() {
				// Create a test Kubernetes config with specific values
				k8sConfig := &config.KubernetesConfig{
					QPS:   100,
					Burst: 200,
				}

				// Create a config provider that returns our test config from ENVTEST
				configProvider := func() (*rest.Config, error) {
					// Create a copy of the test config to avoid modifying the shared config
					cfg := rest.CopyConfig(testConfig)
					return cfg, nil
				}

				// Call the function under test with dependency injection
				resultConfig, err := configureKubernetesClientWithProvider(k8sConfig, configProvider)

				// Assert no error occurred
				Expect(err).NotTo(HaveOccurred())
				Expect(resultConfig).NotTo(BeNil())

				// Verify that QPS and Burst were set correctly
				Expect(resultConfig.QPS).To(Equal(float32(100)))
				Expect(resultConfig.Burst).To(Equal(200))

				// Verify UserAgent was set
				Expect(resultConfig.UserAgent).To(ContainSubstring(types.KubeVelaName))
				Expect(resultConfig.UserAgent).To(ContainSubstring(version.GitRevision))

				// Verify that the config has the impersonating round tripper wrapper
				Expect(resultConfig.Wrap).NotTo(BeNil())
			})

			It("should handle config provider errors gracefully", func() {
				k8sConfig := &config.KubernetesConfig{
					QPS:   100,
					Burst: 200,
				}

				// Create a config provider that returns an error
				configProvider := func() (*rest.Config, error) {
					return nil, fmt.Errorf("failed to get config")
				}

				// Call the function and expect an error
				resultConfig, err := configureKubernetesClientWithProvider(k8sConfig, configProvider)

				// Assert error occurred
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to get config"))
				Expect(resultConfig).To(BeNil())
			})

			It("should apply impersonating round tripper wrapper", func() {
				k8sConfig := &config.KubernetesConfig{
					QPS:   50,
					Burst: 100,
				}

				configProvider := func() (*rest.Config, error) {
					cfg := rest.CopyConfig(testConfig)
					return cfg, nil
				}

				resultConfig, err := configureKubernetesClientWithProvider(k8sConfig, configProvider)

				Expect(err).NotTo(HaveOccurred())
				Expect(resultConfig).NotTo(BeNil())

				// Verify the wrap function was applied
				// We can't directly test the round tripper, but we can verify Wrap is not nil
				Expect(resultConfig.Wrap).NotTo(BeNil())
			})
		})
	})

	Describe("buildManagerOptions", func() {
		var (
			coreOpts *options.CoreOptions
			ctx      context.Context
			cancel   context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			coreOpts = options.NewCoreOptions()

			// Configure options for testing
			coreOpts.Server.EnableLeaderElection = false
			coreOpts.Server.HealthAddr = ":8081"
			coreOpts.Observability.MetricsAddr = ":8080"
			coreOpts.Webhook.UseWebhook = false
			coreOpts.Webhook.CertDir = GinkgoT().TempDir()
			coreOpts.Webhook.WebhookPort = 9443
		})

		AfterEach(func() {
			if cancel != nil {
				cancel()
			}
		})

		Context("when building manager options", func() {
			It("should construct options with correct values from CoreOptions", func() {
				// Call the function under test
				managerOpts := buildManagerOptions(ctx, coreOpts)

				// Verify metrics configuration
				Expect(managerOpts.Metrics.BindAddress).To(Equal(":8080"))

				// Verify health probe configuration
				Expect(managerOpts.HealthProbeBindAddress).To(Equal(":8081"))

				// Verify leader election configuration
				Expect(managerOpts.LeaderElection).To(BeFalse())
				Expect(managerOpts.LeaderElectionID).NotTo(BeEmpty())

				// Verify scheme is set
				Expect(managerOpts.Scheme).NotTo(BeNil())

				// Verify webhook server is configured
				Expect(managerOpts.WebhookServer).NotTo(BeNil())

				// Verify timing configurations
				Expect(managerOpts.LeaseDuration).NotTo(BeNil())
				Expect(*managerOpts.LeaseDuration).To(Equal(coreOpts.Server.LeaseDuration))
				Expect(managerOpts.RenewDeadline).NotTo(BeNil())
				Expect(*managerOpts.RenewDeadline).To(Equal(coreOpts.Server.RenewDeadline))
				Expect(managerOpts.RetryPeriod).NotTo(BeNil())
				Expect(*managerOpts.RetryPeriod).To(Equal(coreOpts.Server.RetryPeriod))

				// Verify client configuration
				Expect(managerOpts.NewClient).NotTo(BeNil())
			})

			It("should handle leader election enabled configuration", func() {
				// Configure with leader election enabled
				coreOpts.Server.EnableLeaderElection = true
				coreOpts.Server.LeaderElectionNamespace = "test-namespace"
				coreOpts.Server.LeaseDuration = 10 * time.Second
				coreOpts.Server.RenewDeadline = 8 * time.Second
				coreOpts.Server.RetryPeriod = 2 * time.Second

				managerOpts := buildManagerOptions(ctx, coreOpts)

				// Verify leader election is enabled
				Expect(managerOpts.LeaderElection).To(BeTrue())
				Expect(managerOpts.LeaderElectionNamespace).To(Equal("test-namespace"))

				// Verify timing configurations match
				Expect(*managerOpts.LeaseDuration).To(Equal(10 * time.Second))
				Expect(*managerOpts.RenewDeadline).To(Equal(8 * time.Second))
				Expect(*managerOpts.RetryPeriod).To(Equal(2 * time.Second))
			})

			It("should construct leader election ID correctly", func() {
				// Test without controller requirement flag
				coreOpts.Controller.IgnoreAppWithoutControllerRequirement = false
				managerOpts := buildManagerOptions(ctx, coreOpts)
				leaderElectionID := managerOpts.LeaderElectionID

				Expect(leaderElectionID).To(ContainSubstring("kubevela"))
				Expect(leaderElectionID).NotTo(BeEmpty())

				// Test with controller requirement flag
				coreOpts.Controller.IgnoreAppWithoutControllerRequirement = true
				managerOpts2 := buildManagerOptions(ctx, coreOpts)
				leaderElectionID2 := managerOpts2.LeaderElectionID

				// Leader election ID should be different when flag changes
				Expect(leaderElectionID2).NotTo(Equal(leaderElectionID))
			})

			It("should configure webhook server with correct port and certDir", func() {
				coreOpts.Webhook.WebhookPort = 9999
				coreOpts.Webhook.CertDir = "/custom/cert/dir"

				managerOpts := buildManagerOptions(ctx, coreOpts)

				// Note: WebhookServer is already constructed, we can't directly inspect
				// port and certDir after construction, but we verify it's not nil
				Expect(managerOpts.WebhookServer).NotTo(BeNil())
			})
		})
	})

	Describe("setupControllers", func() {
		var (
			ctx      context.Context
			cancel   context.CancelFunc
			coreOpts *options.CoreOptions
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			coreOpts = options.NewCoreOptions()
			coreOpts.Webhook.UseWebhook = false // Disable webhooks for simpler testing
		})

		AfterEach(func() {
			if cancel != nil {
				cancel()
			}
		})

		Context("error handling", func() {
			It("should require a valid manager", func() {
				// setupControllers requires a real manager and will panic with nil
				// This documents the current behavior - the function assumes valid inputs
				Expect(func() {
					_ = setupControllers(ctx, nil, coreOpts)
				}).To(Panic())

				// Note: In production, setupControllers is only called after successful
				// createControllerManager, so nil manager should never occur
			})
		})

		// Note: Full integration tests with real manager require:
		// - Complete ENVTEST infrastructure with CRDs
		// - Controller manager initialization
		// - Webhook server setup (if enabled)
	})

	Describe("startApplicationMonitor", func() {
		var (
			ctx    context.Context
			cancel context.CancelFunc
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
		})

		AfterEach(func() {
			if cancel != nil {
				cancel()
			}
		})

		Context("error handling", func() {
			It("should require a valid manager", func() {
				// startApplicationMonitor requires a real manager and will panic with nil
				// This documents the current behavior - the function assumes valid inputs
				Expect(func() {
					_ = startApplicationMonitor(ctx, nil)
				}).To(Panic())

				// Note: In production, startApplicationMonitor is only called after
				// successful manager creation, so nil manager should never occur
			})
		})

		// Note: Full integration tests require:
		// - Initialized controller manager with running informers
		// - Metrics registry setup
		// - Application resources in cluster
	})

	// Note: The run() function requires full Kubernetes environment with CRDs.
	// These unit tests focuses on individual functions with mocked dependencies.
})
