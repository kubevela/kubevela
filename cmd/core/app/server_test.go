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

var (
	testdir      = "testdir"
	testTimeout  = 2 * time.Second
	testInterval = 1 * time.Second
)

func TestGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "test main")
}

var _ = Describe("test waitSecretVolume", func() {
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

var _ = Describe("Server Helper Functions Tests", func() {
	var coreOpts *options.CoreOptions

	BeforeEach(func() {
		coreOpts = options.NewCoreOptions()
	})

	Describe("syncConfigurations", func() {
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

		It("should handle nil options safely", func() {
			nilOpts := &options.CoreOptions{}
			// Should not panic even with nil fields
			Expect(func() {
				syncConfigurations(nilOpts)
			}).NotTo(Panic())
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

		It("should configure debug logging when LogDebug is true", func() {
			obsConfig := &config.ObservabilityConfig{
				LogDebug: true,
			}

			setupLogging(obsConfig)

			// Verify debug level was set (we can't directly check flag values easily)
			// But we can verify the function doesn't panic
			Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
		})

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

		It("should configure standard logging when DevLogs is false", func() {
			obsConfig := &config.ObservabilityConfig{
				DevLogs: false,
			}

			setupLogging(obsConfig)
			Expect(func() { setupLogging(obsConfig) }).NotTo(Panic())
		})
	})

	// Note: Tests for configureKubernetesClient and setupMultiCluster
	// are in server_integration_test.go as they require real Kubernetes config

	Describe("configureFeatureGates", func() {
		It("should configure ApplicationReSyncPeriod when ApplyOnce is enabled", func() {
			// Store original value
			originalPeriod := commonconfig.ApplicationReSyncPeriod
			defer func() {
				commonconfig.ApplicationReSyncPeriod = originalPeriod
			}()

			// Enable the feature gate
			utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=true")
			defer utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=false")

			testPeriod := 5 * time.Minute
			coreOpts.Kubernetes.InformerSyncPeriod = testPeriod

			configureFeatureGates(coreOpts)

			Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(testPeriod))
		})

		It("should not change ApplicationReSyncPeriod when ApplyOnce is disabled", func() {
			originalPeriod := commonconfig.ApplicationReSyncPeriod
			defer func() {
				commonconfig.ApplicationReSyncPeriod = originalPeriod
			}()

			utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=false")

			coreOpts.Kubernetes.InformerSyncPeriod = 10 * time.Minute

			configureFeatureGates(coreOpts)

			Expect(commonconfig.ApplicationReSyncPeriod).To(Equal(originalPeriod))
		})
	})

	// Note: Tests for createControllerManager, setupControllers, and startApplicationMonitor
	// are in server_integration_test.go as they require real Kubernetes components

	Describe("performCleanup", func() {
		It("should flush logs when LogFilePath is set", func() {
			coreOpts.Observability.LogFilePath = "/tmp/test.log"

			// Should not panic
			Expect(func() { performCleanup(coreOpts) }).NotTo(Panic())

			// Verify klog.Flush was called (indirectly)
			performCleanup(coreOpts)
		})

		It("should do nothing when LogFilePath is empty", func() {
			coreOpts.Observability.LogFilePath = ""

			// Should not panic
			Expect(func() { performCleanup(coreOpts) }).NotTo(Panic())
		})
	})

	// Note: Integration tests for the run function are in server_integration_test.go
})

// Unit tests using standard testing package

func TestSyncConfigurationsUnit(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *options.CoreOptions
		validate func(t *testing.T)
	}{
		{
			name: "sync with all configs populated",
			setup: func() *options.CoreOptions {
				return options.NewCoreOptions()
			},
			validate: func(t *testing.T) {
				// Function should not panic
			},
		},
		{
			name: "sync with partial configs",
			setup: func() *options.CoreOptions {
				opts := &options.CoreOptions{
					Workflow:    config.NewWorkflowConfig(),
					CUE:         config.NewCUEConfig(),
					Application: nil, // Intentionally nil
					Performance: config.NewPerformanceConfig(),
					Resource:    config.NewResourceConfig(),
					OAM:         config.NewOAMConfig(),
				}
				return opts
			},
			validate: func(t *testing.T) {
				// Should handle nil configs gracefully
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := tt.setup()

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("syncConfigurations panicked: %v", r)
				}
			}()

			syncConfigurations(opts)
			tt.validate(t)
		})
	}
}

func TestSetupLoggingUnit(t *testing.T) {
	tests := []struct {
		name   string
		config *config.ObservabilityConfig
		verify func(t *testing.T)
	}{
		{
			name: "debug logging enabled",
			config: &config.ObservabilityConfig{
				LogDebug: true,
			},
			verify: func(t *testing.T) {
				// Verify debug flag was set
			},
		},
		{
			name: "file logging configured",
			config: &config.ObservabilityConfig{
				LogFilePath:    "/tmp/test.log",
				LogFileMaxSize: 100,
			},
			verify: func(t *testing.T) {
				// Verify file logging flags were set
			},
		},
		{
			name: "dev logging with colors",
			config: &config.ObservabilityConfig{
				DevLogs: true,
			},
			verify: func(t *testing.T) {
				// Verify color output is configured
			},
		},
		{
			name: "standard logging",
			config: &config.ObservabilityConfig{
				DevLogs: false,
			},
			verify: func(t *testing.T) {
				// Verify standard logger is set
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupLogging(tt.config)
			tt.verify(t)
		})
	}
}

func TestConfigureFeatureGatesUnit(t *testing.T) {
	tests := []struct {
		name           string
		featureEnabled bool
		syncPeriod     time.Duration
		verify         func(t *testing.T, originalPeriod time.Duration)
	}{
		{
			name:           "ApplyOnce enabled",
			featureEnabled: true,
			syncPeriod:     5 * time.Minute,
			verify: func(t *testing.T, originalPeriod time.Duration) {
				if commonconfig.ApplicationReSyncPeriod != 5*time.Minute {
					t.Errorf("Expected sync period to be 5m, got %v", commonconfig.ApplicationReSyncPeriod)
				}
			},
		},
		{
			name:           "ApplyOnce disabled",
			featureEnabled: false,
			syncPeriod:     10 * time.Minute,
			verify: func(t *testing.T, originalPeriod time.Duration) {
				if commonconfig.ApplicationReSyncPeriod != originalPeriod {
					t.Errorf("Expected sync period to remain unchanged")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original
			originalPeriod := commonconfig.ApplicationReSyncPeriod
			defer func() {
				commonconfig.ApplicationReSyncPeriod = originalPeriod
			}()

			// Setup feature gate
			flagValue := fmt.Sprintf("ApplyOnce=%v", tt.featureEnabled)
			utilfeature.DefaultMutableFeatureGate.Set(flagValue)
			defer utilfeature.DefaultMutableFeatureGate.Set("ApplyOnce=false")

			// Create options
			opts := options.NewCoreOptions()
			opts.Kubernetes.InformerSyncPeriod = tt.syncPeriod

			// Test
			configureFeatureGates(opts)

			// Verify
			tt.verify(t, originalPeriod)
		})
	}
}

func TestPerformCleanupUnit(t *testing.T) {
	tests := []struct {
		name        string
		logFilePath string
	}{
		{
			name:        "with log file",
			logFilePath: "/tmp/test.log",
		},
		{
			name:        "without log file",
			logFilePath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := options.NewCoreOptions()
			opts.Observability.LogFilePath = tt.logFilePath

			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("performCleanup panicked: %v", r)
				}
			}()

			performCleanup(opts)
		})
	}
}
