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

package kubebuilder

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// SetKubebuilderAssetsEnv sets the KUBEBUILDER_ASSETS environment variable
// if it's not already set

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

// GinkgoWriter provides output for test logging
var GinkgoWriter = ginkgo.GinkgoWriter

// DownloadKubebuilderBinaries downloads the required binaries if they're missing
func DownloadKubebuilderBinaries() error {
	// Find project root directory
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Execute the download script
	downloadScript := filepath.Join(projectRoot, "hack", "download-binaries.sh")
	fmt.Fprintf(GinkgoWriter, "Attempting to install using %s\n", downloadScript)
	cmd := exec.Command("bash", downloadScript)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run download script: %w", err)
	}

	// Fix permissions after download
	fixPermissionsScript := filepath.Join(projectRoot, "hack", "fix-permissions.sh")
	fmt.Fprintf(GinkgoWriter, "Fixing permissions using %s\n", fixPermissionsScript)
	cmd = exec.Command("bash", fixPermissionsScript)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to fix permissions: %w", err)
	}

	return nil
}

// findProjectRoot tries to find the project root directory
func findProjectRoot() (string, error) {
	// Start with the current file's directory
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to get the current filename")
	}

	dir := filepath.Dir(filename)

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the root without finding go.mod
			break
		}
		dir = parent
	}

	// If we couldn't find it by walking up, try a few common locations
	// Try current working directory
	wd, err := os.Getwd()
	if err == nil {
		// Check if we're in a subdirectory of the project
		if strings.Contains(wd, "kubevela") {
			parts := strings.Split(wd, "kubevela")
			if len(parts) > 0 {
				potential := parts[0] + "kubevela"
				if _, err := os.Stat(filepath.Join(potential, "go.mod")); err == nil {
					return potential, nil
				}
			}
		}
	}

	// Try relative paths that might work in tests
	relPaths := []string{
		"../..",       // From pkg/utils/test/kubebuilder to project root
		"../../..",    // From deeper subdirectory
		"../../../..", // From even deeper subdirectory
	}

	for _, rel := range relPaths {
		absPath, err := filepath.Abs(filepath.Join(dir, rel))
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(absPath, "go.mod")); err == nil {
			return absPath, nil
		}
	}

	return "", fmt.Errorf("could not find project root (no go.mod found)")
}
