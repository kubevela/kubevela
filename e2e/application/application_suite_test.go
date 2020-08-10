package e2e

import (
	"testing"

	"github.com/cloud-native-application/rudrx/e2e"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var rudrPath string

var _ = ginkgo.BeforeSuite(func() {
	p, err := e2e.GetCliBinary()
	rudrPath = p
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
})

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Application Suite")
}
