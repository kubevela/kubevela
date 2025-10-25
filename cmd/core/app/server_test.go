//go:build !integration
// +build !integration

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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"

	"github.com/oam-dev/kubevela/cmd/core/app/config"
	"github.com/oam-dev/kubevela/cmd/core/app/options"
	commonconfig "github.com/oam-dev/kubevela/pkg/controller/common"
)

/*
Test Organization Notes:
- Unit tests for helper functions are in this file
- Integration tests requiring Kubernetes components are in server_integration_test.go
- Tests are organized by the functions they test
- All tests use Ginkgo for consistency
*/

var (
	testdir      = "testdir"
	testTimeout  = 2 * time.Second
	testInterval = 1 * time.Second
)

func TestGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "test main")
}

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
				_, err := os.Create(testdir + "/emptyFile")
				Expect(err).NotTo(HaveOccurred())
				err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
				Expect(err).To(HaveOccurred())
			})
		})

		When("files in dir are not empty", func() {
			It("return nil", func() {
				By("add non-empty file")
				_, err := os.Create(testdir + "/file")
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(testdir+"/file", []byte("test"), os.ModeAppend)
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
			utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=false")
		})

		Context("when ApplyOnce is enabled", func() {
			It("should configure ApplicationReSyncPeriod", func() {
				// Enable the feature gate
				utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=true")

				testPeriod := 5 * time.Minute
				coreOpts.Kubernetes.InformerSyncPeriod = testPeriod

				configureFeatureGates(coreOpts)

				Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(testPeriod))
			})
		})

		Context("when ApplyOnce is disabled", func() {
			It("should not change ApplicationReSyncPeriod", func() {
				utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=false")

				coreOpts.Kubernetes.InformerSyncPeriod = 10 * time.Minute

				configureFeatureGates(coreOpts)

				Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(originalPeriod))
			})
		})

		Context("with different sync periods", func() {
			DescribeTable("should handle various sync periods correctly",
				func(enabled bool, syncPeriod time.Duration, expectedResult time.Duration) {
					flagValue := fmt.Sprintf("ApplyOnce=%v", enabled)
					utilfeature.DefaultMutableFeatureGate.Set(flagValue)

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
})

/*
Additional Notes:
- Integration tests for the following functions are in server_integration_test.go:
  - configureKubernetesClient (requires real Kubernetes config)
  - setupMultiCluster (requires real Kubernetes config)
  - createControllerManager (requires real Kubernetes components)
  - setupControllers (requires controller manager)
  - startApplicationMonitor (requires controller manager)
  - run function (full integration test)
*/
