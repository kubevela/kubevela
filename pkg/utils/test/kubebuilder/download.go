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
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
)

// DownloadKubebuilderBinaries attempts to download the required binaries
// Returns error if download fails
func DownloadKubebuilderBinaries() error {
	// Find project root to locate download script
	projectRoot, err := FindProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Locate download script
	downloadScript := filepath.Join(projectRoot, "hack", "download-binaries.sh")
	if _, err := os.Stat(downloadScript); os.IsNotExist(err) {
		return fmt.Errorf("download script not found at %s", downloadScript)
	}

	// Run the download script
	fmt.Fprintf(GinkgoWriter, "Attempting to install using %s\n", downloadScript)
	cmd := exec.Command("bash", downloadScript)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run download script: %w", err)
	}

	// Verify binaries were installed
	if err := CheckBinaries(); err != nil {
		return fmt.Errorf("binaries not properly installed: %w", err)
	}

	// Fix permissions on binaries
	fixPermissionsScript := filepath.Join(projectRoot, "hack", "fix-permissions.sh")
	if _, err := os.Stat(fixPermissionsScript); os.IsNotExist(err) {
		// Manually fix permissions if script doesn't exist
		if err := MakeAllBinariesExecutable(); err != nil {
			return fmt.Errorf("failed to make binaries executable: %w", err)
		}
	} else {
		cmd = exec.Command("bash", fixPermissionsScript)
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run fix-permissions script: %w", err)
		}
	}

	return nil
}

// FindProjectRoot tries to find the project root directory by looking for go.mod file
func FindProjectRoot() (string, error) {
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

	// If we couldn't find it by walking up, try relative to GOPATH
	gopath := os.Getenv("GOPATH")
	if gopath != "" {
		for _, p := range filepath.SplitList(gopath) {
			dir := filepath.Join(p, "src", "github.com", "oam-dev", "kubevela")
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return dir, nil
			}
		}
	}

	// Try relative paths that might work in tests
	relPaths := []string{
		"../../..",    // From pkg/utils/test/kubebuilder to project root
		"../../../..", // From deeper subdirectory
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

	// Finally, try to use the working directory or its parent
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("unable to get working directory: %w", err)
	}

	// Check current working directory
	if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
		return wd, nil
	}

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

	return "", fmt.Errorf("could not find project root")
}
