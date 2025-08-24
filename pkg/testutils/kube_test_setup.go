/*
Copyright 2023 The KubeVela Authors.

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

package testutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// KubeTestSetup provides a consistent way to set up Kubernetes tests
type KubeTestSetup struct {
	// Embedded test environment
	Environment *envtest.Environment
	
	// Configuration and clients
	Config *rest.Config
	Client client.Client
	
	// Settings
	CRDPaths        []string
	UseExistingCluster bool
	
	// Test state
	started bool
}

// NewKubeTestSetup creates a new test setup with default configuration
func NewKubeTestSetup() *KubeTestSetup {
	// Setup KUBEBUILDER_ASSETS if not set
	setupKubebuilderAssets()
	
	return &KubeTestSetup{
		Environment: &envtest.Environment{
			UseExistingCluster: ptr.To(false),
		},
		CRDPaths: []string{},
		UseExistingCluster: false,
	}
}

// WithCRDPath adds a CRD path to the test environment
func (k *KubeTestSetup) WithCRDPath(path string) *KubeTestSetup {
	k.CRDPaths = append(k.CRDPaths, path)
	return k
}

// WithExistingCluster configures the test to use an existing cluster
func (k *KubeTestSetup) WithExistingCluster(use bool) *KubeTestSetup {
	k.UseExistingCluster = use
	return k
}

// Start starts the test environment and initializes clients
func (k *KubeTestSetup) Start() error {
	if k.started {
		return nil
	}
	
	// Configure the test environment
	k.Environment.UseExistingCluster = ptr.To(k.UseExistingCluster)
	k.Environment.CRDDirectoryPaths = k.CRDPaths
	
	// Start the environment
	var err error
	k.Config, err = k.Environment.Start()
	if err != nil {
		return fmt.Errorf("failed to start test environment: %w", err)
	}
	
	// Create a client
	k.Client, err = client.New(k.Config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	
	k.started = true
	return nil
}

// StartOrDie starts the test environment or fails the test
func (k *KubeTestSetup) StartOrDie() {
	err := k.Start()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, k.Config).NotTo(BeNil())
	ExpectWithOffset(1, k.Client).NotTo(BeNil())
}

// Stop stops the test environment
func (k *KubeTestSetup) Stop() error {
	if !k.started {
		return nil
	}
	
	if k.Environment != nil {
		return k.Environment.Stop()
	}
	
	return nil
}

// StopOrDie stops the test environment or fails the test
func (k *KubeTestSetup) StopOrDie() {
	if k.started && k.Environment != nil {
		err := k.Environment.Stop()
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}
}

// SetupForSuite provides a standard setup for Ginkgo test suites
func (k *KubeTestSetup) SetupForSuite() {
	// Define BeforeSuite and AfterSuite hooks
	BeforeSuite(func() {
		By("Setting up the test environment")
		k.StartOrDie()
	})
	
	AfterSuite(func() {
		By("Tearing down the test environment")
		k.StopOrDie()
	})
}

// setupKubebuilderAssets sets up the KUBEBUILDER_ASSETS environment variable
func setupKubebuilderAssets() {
	// Check if KUBEBUILDER_ASSETS is already set
	if os.Getenv("KUBEBUILDER_ASSETS") != "" {
		return
	}
	
	// Common locations to check
	commonPaths := []string{
		"/usr/local/kubebuilder/bin",
		filepath.Join(os.Getenv("HOME"), "kubebuilder", "bin"),
		filepath.Join(os.Getenv("HOME"), ".kubebuilder", "bin"),
	}
	
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			// Check if etcd exists in this directory
			if _, err := os.Stat(filepath.Join(path, "etcd")); err == nil {
				os.Setenv("KUBEBUILDER_ASSETS", path)
				return
			}
		}
	}
	
	// Try to download the binaries if not found
	downloadBinaries()
}

// downloadBinaries attempts to download the required binaries
func downloadBinaries() {
	// Execute the download script if it exists
	scriptPath := "hack/download-test-binaries.sh"
	if _, err := os.Stat(scriptPath); err == nil {
		cmd := exec.Command("/bin/bash", scriptPath)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(GinkgoWriter, "Warning: Failed to download test binaries: %v\n", err)
		}
	}
}
