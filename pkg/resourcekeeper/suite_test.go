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

package resourcekeeper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/testutils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnvSetup *testutils.TestEnv
var ctx context.Context
var cancel context.CancelFunc
var testScheme = runtime.NewScheme()

func TestResourceKeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ResourceKeeper Suite")
}

var _ = BeforeSuite(func() {
	By("Bootstrapping test environment")

	// Check for required binaries before starting envtest
	requiredBinaries := []string{"etcd", "kube-apiserver", "kubectl"}
	for _, bin := range requiredBinaries {
		binPath := filepath.Join("/usr/local/kubebuilder/bin", bin)
		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			Fail(fmt.Sprintf("Required binary %s not found at %s. Please install kubebuilder and ensure all binaries are present.", bin, binPath))
		}
	}

	ctx, cancel = context.WithCancel(context.Background())

	var err error

	// Use our robust test environment setup
	testEnvSetup = testutils.NewTestEnv(common.Scheme, false)
	testEnvSetup.WithCRDPath(
		filepath.Join("../../..", "charts/vela-core/crds"), // this has all the required CRDs
	)

	err = testEnvSetup.Start()
	Expect(err).NotTo(HaveOccurred())

	// Setup for backward compatibility
	cfg = testEnvSetup.Config
	k8sClient = testEnvSetup.Client

	By("Setup test namespace")
	ns := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "vela-system"}}
	Expect(k8sClient.Create(ctx, &ns)).Should(SatisfyAny(BeNil(), &AlreadyExistMatcher{}))
})

var _ = AfterSuite(func() {
	// Cancel the context to clean up resources
	if cancel != nil {
		cancel()
	}

	// Safe stop of the test environment
	if testEnvSetup != nil {
		By("Tearing down the test environment")
		err := testEnvSetup.Stop()
		Expect(err).NotTo(HaveOccurred())
	}
})

// AlreadyExistMatcher matches the error returned by k8s when a resource already exists
type AlreadyExistMatcher struct{}

// Match checks if the error indicates that a resource already exists
func (matcher *AlreadyExistMatcher) Match(actual interface{}) (success bool, err error) {
	if actual == nil {
		return false, nil
	}
	actualError, ok := actual.(error)
	if !ok {
		return false, nil
	}
	return actualError != nil && actualError.Error() == "namespace \"vela-system\" already exists", nil
}

// FailureMessage returns a failure message
func (matcher *AlreadyExistMatcher) FailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected error to indicate resource already exists, got '%v'", actual)
}

// NegatedFailureMessage returns a negated failure message
func (matcher *AlreadyExistMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected error not to indicate resource already exists, got '%v'", actual)
}

func init() {
	_ = scheme.AddToScheme(testScheme)
	_ = v1beta1.AddToScheme(testScheme)
}
