package integration_test

import (
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloud-native-application/rudrx/pkg/server"
)

var restServer http.Handler

var _ = BeforeSuite(func() {
	restServer = server.SetupRoute()
})

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}
