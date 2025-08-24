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

// Package kubebuilder provides utilities for working with kubebuilder in tests
package kubebuilder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

// RequiredBinaries lists the binaries that are required by kubebuilder envtest
var RequiredBinaries = []string{"etcd", "kube-apiserver", "kubectl"}

// CheckBinaries checks if required kubebuilder binaries exist and returns an error if any are missing
func CheckBinaries() error {
	// Check if KUBEBUILDER_ASSETS is set
	kubeBuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
	if kubeBuilderAssets == "" {
		kubeBuilderAssets = "/usr/local/kubebuilder/bin"
	}

	// Check required binaries
	for _, binary := range RequiredBinaries {
		binaryPath := filepath.Join(kubeBuilderAssets, binary)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return fmt.Errorf("required binary %s not found at %s", binary, binaryPath)
		}
	}
	return nil
}

// SkipIfBinariesMissing checks if required kubebuilder binaries exist and skips the test if any are missing
// It attempts to download the binaries if they're missing
func SkipIfBinariesMissing() {
	kubebuilderAssetsEnv := os.Getenv("KUBEBUILDER_ASSETS")
	if kubebuilderAssetsEnv == "" {
		Skip("KUBEBUILDER_ASSETS environment variable not set")
		return
	}

	// Check if binaries exist
	missing := false
	for _, binary := range RequiredBinaries {
		binPath := filepath.Join(kubebuilderAssetsEnv, binary)
		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			fmt.Fprintf(GinkgoWriter, "Required binary %s not found at %s\n", binary, binPath)
			missing = true
		} else {
			// Make sure the binary is executable
			if err := os.Chmod(binPath, 0755); err != nil {
				fmt.Fprintf(GinkgoWriter, "Failed to make %s executable: %v\n", binary, err)
			}
		}
	}

	// Try to download missing binaries
	if missing {
		fmt.Fprintf(GinkgoWriter, "Required binaries missing: kube-apiserver\n")
		// Try to download the binaries
		err := DownloadKubebuilderBinaries()
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "Failed to download binaries: %v\n", err)
			fmt.Fprintf(GinkgoWriter, "Please install the required binaries manually or run: bash ../../hack/download-binaries.sh\n")
			Skip(fmt.Sprintf("Required binaries missing and failed to download them: %v", err))
		}

		// Verify binaries exist after download
		for _, binary := range RequiredBinaries {
			binPath := filepath.Join(kubebuilderAssetsEnv, binary)
			if _, err := os.Stat(binPath); os.IsNotExist(err) {
				Skip(fmt.Sprintf("Required binary %s still missing after attempted download", binary))
				return
			}

			// Check if binary is executable
			cmd := exec.Command(binPath, "--version")
			if err := cmd.Run(); err != nil {
				Skip(fmt.Sprintf("Required binary %s is not executable after download: %v", binary, err))
				return
			}
		}
	}
}

// SetKubebuilderAssetsEnv ensures KUBEBUILDER_ASSETS is set properly
func SetKubebuilderAssetsEnv() {
	// Check if KUBEBUILDER_ASSETS is already set
	if _, exists := os.LookupEnv("KUBEBUILDER_ASSETS"); !exists {
		// Try to use the binaries from /usr/local/kubebuilder/bin
		if _, err := os.Stat("/usr/local/kubebuilder/bin"); err == nil {
			os.Setenv("KUBEBUILDER_ASSETS", "/usr/local/kubebuilder/bin")
		}
	}
}
