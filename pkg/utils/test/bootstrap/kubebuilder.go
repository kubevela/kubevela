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

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
	kubebuilderutil "github.com/oam-dev/kubevela/pkg/utils/test/kubebuilder"
)

var (
	env             client.Client
	bootstrapLogger = common.NewLogger("bootstrap")
)

// CheckKubeBuilderBinaries checks if the required kubebuilder binaries exist
func CheckKubeBuilderBinaries() error {
	// Check if KUBEBUILDER_ASSETS is set
	kubeBuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
	if kubeBuilderAssets == "" {
		kubeBuilderAssets = "/usr/local/kubebuilder/bin"
	}

	// Check required binaries
	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}
	for _, binary := range requiredBinaries {
		binaryPath := filepath.Join(kubeBuilderAssets, binary)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("required binary %s not found at %s", binary, binaryPath)
		}
	}
	return nil
}

// setUpTestEnv sets up a test environment for testing
func setUpTestEnv() (*envtest.Environment, *envtest.Server, error) {
	// Create a new test environment
	env := &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "charts", "vela-core", "crds"),
		},
	}

	// Start the test environment
	cfg, err := env.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start test environment: %w", err)
	}

	// Create a server to handle the requests
	s := envtest.NewServer(cfg)

	return env, s, nil
}

// InitKubeBuilderForTest init and start kubebuilder for test
func InitKubeBuilderForTest() common.CleanupFunc {
	bootstrapLogger.LogInfo("Bootstrapping Test Environment")

	By("Bootstrapping Test Environment")

	// Set KUBEBUILDER_ASSETS environment variable if not already set
	kubebuilderutil.SetKubebuilderAssetsEnv()

	// Skip if kubebuilder binaries are missing
	kubebuilderutil.SkipIfBinariesMissing()

	testEnv, s, err := setUpTestEnv()
	if err != nil {
		bootstrapLogger.LogInfo("Unable to bootstrap the kubebuilder", "errMessage", err.Error())
		Skip(fmt.Sprintf("Failed to bootstrap kubebuilder: %v", err))
		return func() {}
	}

	// Start the controlplane
	By("Starting control plane")
	startErr := testEnv.Start()
	if startErr != nil {
		bootstrapLogger.LogInfo("Unable to start the controlplane", "errMessage", startErr.Error())
		Skip(fmt.Sprintf("Failed to start control plane: %v", startErr))
		return func() {}
	}

	By("Creating the client")
	cl, err := client.New(testEnv.Config, client.Options{})
	if err != nil {
		bootstrapLogger.LogInfo("Unable to create a client", "errMessage", err.Error())
		// Ensure we stop the environment that was started
		if testEnv != nil {
			_ = testEnv.Stop()
		}
		if s != nil {
			s.Stop()
		}
		Skip(fmt.Sprintf("Failed to create client: %v", err))
		return func() {}
	}

	env = cl

	return func() {
		By("Tearing Down the Test Environment")
		bootstrapLogger.LogInfo("Tearing Down the Test Environment")
		if testEnv != nil {
			_ = testEnv.Stop()
		}
		if s != nil {
			s.Stop()
		}
	}
}
