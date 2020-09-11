package e2e

import (
	"testing"

	"github.com/oam-dev/kubevela/e2e"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var _ = ginkgo.BeforeSuite(func() {
	e2e.BeforeSuit()
}, 30)

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Setup Suite")
}
