package e2e

import (
	"fmt"
	"testing"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var rudrPath string

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	_, err := e2e.GetCliBinary()
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	e2e.Exec("vela system:init")
	return nil
}, func(data []byte) {
	fmt.Println("SynchronizedBeforeSuite 2")
})

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "System Suite")
}
