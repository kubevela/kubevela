package e2e

import (
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

func TestEnv(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Env Suite")
}
