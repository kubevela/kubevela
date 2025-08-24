/*
Copyright 2020-2022 The KubeVela Authors.

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

package multicluster

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/cluster-gateway/pkg/apis/cluster/v1alpha1"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}

// findBinaryPath looks for the binary in multiple possible locations
func findBinaryPath(binary string) (string, bool) {
	// Check if KUBEBUILDER_ASSETS is set
	if assets := os.Getenv("KUBEBUILDER_ASSETS"); assets != "" {
		path := filepath.Join(assets, binary)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}

	// Check in /usr/local/kubebuilder/bin
	path := filepath.Join("/usr/local/kubebuilder/bin", binary)
	if _, err := os.Stat(path); err == nil {
		return path, true
	}

	// Check in PATH
	if path, err := exec.LookPath(binary); err == nil {
		return path, true
	}

	// Not found
	return "", false
}

// setupEnvTest ensures binaries are available and properly configured
func setupEnvTest() {
	// Check for required binaries
	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}
	missingBinaries := []string{}

	for _, bin := range requiredBinaries {
		if _, found := findBinaryPath(bin); !found {
			missingBinaries = append(missingBinaries, bin)
		}
	}

	if len(missingBinaries) > 0 {
		// Check if the download script exists
		scriptPath := filepath.Join("..", "..", "hack", "download-binaries.sh")
		if _, err := os.Stat(scriptPath); err == nil {
			fmt.Printf("Required binaries missing: %s\n", strings.Join(missingBinaries, ", "))
			fmt.Printf("Attempting to install using %s\n", scriptPath)

			// Try to run the script
			cmd := exec.Command("bash", scriptPath)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				Fail(fmt.Sprintf("Failed to run download script: %v\n\nPlease install the required binaries manually or run: bash %s", err, scriptPath))
			}

			// Check again after installation
			stillMissing := []string{}
			for _, bin := range missingBinaries {
				if _, found := findBinaryPath(bin); !found {
					stillMissing = append(stillMissing, bin)
				}
			}

			if len(stillMissing) > 0 {
				Fail(fmt.Sprintf("Still missing required binaries after installation: %s\nPlease install them manually", strings.Join(stillMissing, ", ")))
			}

			// If KUBEBUILDER_ASSETS is not set, set it
			if os.Getenv("KUBEBUILDER_ASSETS") == "" {
				fmt.Println("Setting KUBEBUILDER_ASSETS to /usr/local/kubebuilder/bin")
				os.Setenv("KUBEBUILDER_ASSETS", "/usr/local/kubebuilder/bin")
			}
		} else {
			Fail(fmt.Sprintf("Required binaries not found: %s\nPlease install them or run hack/download-binaries.sh", strings.Join(missingBinaries, ", ")))
		}
	}
}

var _ = BeforeSuite(func() {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment for utils test")

	// Ensure required binaries are available
	setupEnvTest()

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
		CRDDirectoryPaths:        []string{"./testdata"},
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	Expect(v1alpha1.AddToScheme(common.Scheme)).Should(Succeed())
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		By("tearing down the test environment")
		err := testEnv.Stop()
		if err != nil {
			GinkgoWriter.Printf("Error stopping test environment: %v\n", err)
		}
	}
})
