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

package utils

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}

var _ = BeforeSuite(func() {
	rand.Seed(time.Now().UnixNano())
	By("bootstrapping test environment for utils test")
	// Check if KUBEBUILDER_ASSETS is already set
	if _, exists := os.LookupEnv("KUBEBUILDER_ASSETS"); !exists {
		// Try to use the binaries from /usr/local/kubebuilder/bin
		if _, err := os.Stat("/usr/local/kubebuilder/bin"); err == nil {
			os.Setenv("KUBEBUILDER_ASSETS", "/usr/local/kubebuilder/bin")
		}
	}

	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}
	for _, bin := range requiredBinaries {
		binPath := filepath.Join("/usr/local/kubebuilder/bin", bin)
		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			Fail(fmt.Sprintf("Required binary %s not found at %s. Please install kubebuilder and ensure all binaries are present.", bin, binPath))
		}
	}

	testEnv = &envtest.Environment{
		ControlPlaneStartTimeout: time.Minute * 3,
		ControlPlaneStopTimeout:  time.Minute,
		UseExistingCluster:       ptr.To(false),
	}

	By("start kube test env")
	var err error
	cfg, err = testEnv.Start()
	Expect(err).ShouldNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	By("new kube client")
	cfg.Timeout = time.Minute * 2
	k8sClient, err = client.New(cfg, client.Options{Scheme: common.Scheme})
	Expect(err).Should(BeNil())
	Expect(k8sClient).ToNot(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv != nil {
		_ = testEnv.Stop()
	}
})
