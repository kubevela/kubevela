package component

import (
	"testing"

	"github.com/cloud-native-application/rudrx/pkg/test"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var rudrPath string

var _ = ginkgo.BeforeSuite(func() {
	p, err := test.GetCliBinary()
	rudrPath = p
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
})

func TestComponent(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Command Suite")
}
