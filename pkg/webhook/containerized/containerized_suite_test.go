package containerized_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestContainerized(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Containerized Suite")
}
