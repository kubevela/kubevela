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

package util

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CheckKubebuilderBinaries checks if required kubebuilder binaries exist
// and returns an error if any are missing
func CheckKubebuilderBinaries() error {
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

// InstallKubebuilder runs the setup-kubebuilder.sh script to install kubebuilder binaries
func InstallKubebuilder() error {
	// Check if hack/setup-kubebuilder.sh exists
	scriptPath := filepath.Join("hack", "setup-kubebuilder.sh")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("setup script not found at %s", scriptPath)
	}
	
	// Make the script executable
	if err := os.Chmod(scriptPath, 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %v", err)
	}
	
	// Run the script
	cmd := exec.Command("/bin/sh", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run setup script: %v\nOutput: %s", err, string(output))
	}
	
	return nil
}
