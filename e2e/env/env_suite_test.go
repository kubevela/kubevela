package e2e

import (
	"testing"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var rudrPath string

var _ = ginkgo.BeforeSuite(func() {
	e2e.BeforeSuit()
})

func TestEnv(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Env Suite")
}
