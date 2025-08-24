package hooks_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	bootstraptesting "github.com/kubevela/pkg/util/test/bootstrap"
)

// checkKubebuilderBinaries checks if required kubebuilder binaries exist
func checkKubebuilderBinaries() error {
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

func TestHooksSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testability Suite")
}

var _ = BeforeSuite(func() {
	// Check if kubebuilder binaries exist before initializing
	if err := checkKubebuilderBinaries(); err != nil {
		Skip(fmt.Sprintf("Skipping tests: %v\nRun hack/setup-kubebuilder.sh to install required binaries", err))
		return
	}

	bootstraptesting.InitKubeBuilderForTest()
})
