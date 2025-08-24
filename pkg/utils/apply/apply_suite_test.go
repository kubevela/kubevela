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

package apply

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	oamcore "github.com/oam-dev/kubevela/apis/core.oam.dev"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var testEnv *envtest.Environment
var cfg *rest.Config
var rawClient client.Client
var k8sApplicator Applicator
var testScheme = runtime.NewScheme()
var ns = "test-apply"
var applyNS corev1.Namespace

func TestApplicator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Applicator Suite")
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
		scriptPath := filepath.Join("..", "..", "..", "hack", "download-binaries.sh")
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
		} else {
			Fail(fmt.Sprintf("Required binaries not found: %s\nPlease install them using hack/download-binaries.sh", strings.Join(missingBinaries, ", ")))
		}
	}

	// If KUBEBUILDER_ASSETS is not set, set it to /usr/local/kubebuilder/bin
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Println("Setting KUBEBUILDER_ASSETS to /usr/local/kubebuilder/bin")
		os.Setenv("KUBEBUILDER_ASSETS", "/usr/local/kubebuilder/bin")
	}
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")

	// Ensure required binaries are available
	setupEnvTest()

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute,
		ControlPlaneStopTimeout:  time.Minute,
		CRDDirectoryPaths: []string{
			filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs,
		},
	}
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ShouldNot(BeNil())

	logf.SetLogger(zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter)))
	Expect(clientgoscheme.AddToScheme(testScheme)).Should(Succeed())
	Expect(oamcore.AddToScheme(testScheme)).Should(Succeed())

	By("Setting up applicator")
	rawClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).ShouldNot(HaveOccurred())
	Expect(rawClient).ShouldNot(BeNil())
	k8sApplicator = NewAPIApplicator(rawClient)

	By("Create test namespace")
	applyNS = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}
	Expect(rawClient.Create(context.Background(), &applyNS)).Should(Succeed())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		_ = testEnv.Stop()
	}
})
