/*
Copyright 2023 The KubeVela Authors.
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
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	coreoam "github.com/oam-dev/kubevela/apis/core.oam.dev"
)

// SetupEnvTest creates a new envtest environment for testing
func SetupEnvTest() (*envtest.Environment, error) {
	// Set up the test environment
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths:        []string{filepath.Join("..", "..", "charts", "vela-core", "crds")},
	}

	// Create scheme
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	
	err = coreoam.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add core.oam.dev scheme: %w", err)
	}

	// Start the test environment
	cfg, err := testEnv.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start test environment: %w", err)
	}

	// Create client
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
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

package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// Scheme is the shared scheme for tests
var Scheme = runtime.NewScheme()

// SetupTestEnv sets up a test environment for testing
func SetupTestEnv() (*envtest.Environment, client.Client, error) {
	// Add standard Kubernetes types to scheme
	err := clientgoscheme.AddToScheme(Scheme)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}

	// Check if kubebuilder binaries exist
	kubeBuilderAssets := os.Getenv("KUBEBUILDER_ASSETS")
	if kubeBuilderAssets == "" {
		kubeBuilderAssets = "/usr/local/kubebuilder/bin"
	}

	// Check required binaries
	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}
	for _, binary := range requiredBinaries {
		binaryPath := filepath.Join(kubeBuilderAssets, binary)
		if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("required binary %s not found at %s", binary, binaryPath)
		}

		// Make the binary executable
		err := os.Chmod(binaryPath, 0755)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to make %s executable: %w", binary, err)
		}
	}

	// Create a new test environment
	testEnv := &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "charts", "vela-core", "crds"),
		},
	}

	// Start the test environment
	cfg, err := testEnv.Start()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start test environment: %w", err)
	}

	// Create a client for the test environment
	k8sClient, err := client.New(cfg, client.Options{Scheme: Scheme})
	if err != nil {
		_ = testEnv.Stop()
		return nil, nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	return testEnv, k8sClient, nil
}
	// Store client in context
	SetClient(k8sClient)

	return testEnv, nil
}

// SetClient sets the client for use in tests
func SetClient(c client.Client) {
	// Implementation depends on how you want to store and retrieve the client
	// This is a placeholder
}

// GetClient gets the client for use in tests
func GetClient() client.Client {
	// Implementation depends on how you want to store and retrieve the client
	// This is a placeholder
	return nil
}
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
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestEnv provides a robust test environment setup
type TestEnv struct {
	EnvTest    *envtest.Environment
	Config     *rest.Config
	Client     client.Client
	Scheme     client.Scheme
	UseExisting bool
}

// NewTestEnv creates a new test environment with sensible defaults
func NewTestEnv(scheme client.Scheme, useExisting bool) *TestEnv {
	return &TestEnv{
		EnvTest: &envtest.Environment{
			ControlPlaneStartTimeout: time.Minute * 3,
			ControlPlaneStopTimeout:  time.Minute,
			UseExistingCluster:       ptr.To(useExisting),
		},
		Scheme: scheme,
		UseExisting: useExisting,
	}
}

// WithCRDPath adds CRD paths to the test environment
func (te *TestEnv) WithCRDPath(crdPaths ...string) *TestEnv {
	te.EnvTest.CRDDirectoryPaths = append(te.EnvTest.CRDDirectoryPaths, crdPaths...)
	return te
}

// Start starts the test environment
func (te *TestEnv) Start() error {
	// Check if we're using etcd directly or need binaries
	if !te.UseExisting {
		// Ensure binaries are available
		if err := te.ensureTestBinaries(); err != nil {
			return fmt.Errorf("failed to ensure test binaries: %w", err)
		}
	}

	// Start the test environment
	var err error
	te.Config, err = te.EnvTest.Start()
	if err != nil {
		return fmt.Errorf("failed to start test environment: %w", err)
	}

	// Create a client with the provided scheme
	te.Config.Timeout = time.Minute * 2
	te.Client, err = client.New(te.Config, client.Options{Scheme: te.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return nil
}

// Stop stops the test environment
func (te *TestEnv) Stop() error {
	if te.EnvTest != nil {
		return te.EnvTest.Stop()
	}
	return nil
}

// ensureTestBinaries checks for required binaries and downloads them if needed
func (te *TestEnv) ensureTestBinaries() error {
	// First check if KUBEBUILDER_ASSETS is set
	assetsPath := os.Getenv("KUBEBUILDER_ASSETS")
	if assetsPath == "" {
		// Try common locations
		commonPaths := []string{
			"/usr/local/kubebuilder/bin",
			filepath.Join(os.Getenv("HOME"), "kubebuilder", "bin"),
			filepath.Join(os.Getenv("HOME"), ".kubebuilder", "bin"),
		}

		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				// Found a potential path, check if etcd exists
				if _, err := os.Stat(filepath.Join(path, "etcd")); err == nil {
					assetsPath = path
					break
				}
			}
		}

		// If still not found, use a temporary directory and download
		if assetsPath == "" {
			var err error
			assetsPath, err = te.downloadTestBinaries()
			if err != nil {
				return fmt.Errorf("failed to download test binaries: %w", err)
			}
		}

		// Set KUBEBUILDER_ASSETS environment variable
		os.Setenv("KUBEBUILDER_ASSETS", assetsPath)
	}

	// Verify binaries exist
	requiredBinaries := []string{"etcd", "kube-apiserver"}
	for _, bin := range requiredBinaries {
		binPath := filepath.Join(assetsPath, bin)
		if _, err := os.Stat(binPath); err != nil {
			return fmt.Errorf("required binary %s not found at %s", bin, binPath)
		}
	}

	return nil
}

// downloadTestBinaries downloads required binaries for testing
func (te *TestEnv) downloadTestBinaries() (string, error) {
	// Create a temporary directory for the binaries
	tempDir, err := os.MkdirTemp("", "kubebuilder-assets")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Download etcd
	if err := te.downloadEtcd(tempDir); err != nil {
		return "", fmt.Errorf("failed to download etcd: %w", err)
	}

	// Download kube-apiserver
	if err := te.downloadKubeAPIServer(tempDir); err != nil {
		return "", fmt.Errorf("failed to download kube-apiserver: %w", err)
	}

	return tempDir, nil
}

// downloadEtcd downloads etcd binaries
func (te *TestEnv) downloadEtcd(destDir string) error {
	etcdVersion := "3.5.7"
	tempFile := filepath.Join(os.TempDir(), "etcd.tar.gz")
	
	// Download etcd tarball
	cmd := exec.Command("curl", "-L", "-o", tempFile, 
		fmt.Sprintf("https://github.com/etcd-io/etcd/releases/download/v%s/etcd-v%s-linux-amd64.tar.gz", etcdVersion, etcdVersion))
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download etcd: %s, %s", err, stderr.String())
	}
	
	// Extract etcd binaries
	extractDir := filepath.Join(os.TempDir(), "etcd-extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return fmt.Errorf("failed to create extraction directory: %w", err)
	}
	
	cmd = exec.Command("tar", "-xzf", tempFile, "-C", extractDir, "--strip-components=1")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract etcd: %s, %s", err, stderr.String())
	}
	
	// Copy etcd and etcdctl to destination
	if err := copyFile(filepath.Join(extractDir, "etcd"), filepath.Join(destDir, "etcd")); err != nil {
		return fmt.Errorf("failed to copy etcd: %w", err)
	}
	
	if err := copyFile(filepath.Join(extractDir, "etcdctl"), filepath.Join(destDir, "etcdctl")); err != nil {
		return fmt.Errorf("failed to copy etcdctl: %w", err)
	}
	
	// Make them executable
	if err := os.Chmod(filepath.Join(destDir, "etcd"), 0755); err != nil {
		return fmt.Errorf("failed to make etcd executable: %w", err)
	}
	
	if err := os.Chmod(filepath.Join(destDir, "etcdctl"), 0755); err != nil {
		return fmt.Errorf("failed to make etcdctl executable: %w", err)
	}
	
	// Clean up
	os.Remove(tempFile)
	os.RemoveAll(extractDir)
	
	return nil
}

// downloadKubeAPIServer downloads kube-apiserver binary
func (te *TestEnv) downloadKubeAPIServer(destDir string) error {
	k8sVersion := "1.26.1"
	apiServerPath := filepath.Join(destDir, "kube-apiserver")
	
	// Download kube-apiserver binary
	cmd := exec.Command("curl", "-L", "-o", apiServerPath, 
		fmt.Sprintf("https://dl.k8s.io/v%s/bin/linux/amd64/kube-apiserver", k8sVersion))
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download kube-apiserver: %s, %s", err, stderr.String())
	}
	
	// Make it executable
	if err := os.Chmod(apiServerPath, 0755); err != nil {
		return fmt.Errorf("failed to make kube-apiserver executable: %w", err)
	}
	
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	
	_, err = io.Copy(destination, source)
	return err
}
