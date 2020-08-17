package e2e

import (
	"testing"

	"github.com/cloud-native-application/rudrx/e2e"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var _ = ginkgo.BeforeSuite(func() {
	e2e.AsyncExec("vela dashboard")
})

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Setup Suite")
}
