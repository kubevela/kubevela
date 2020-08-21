package e2e

import (
	"net/http"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var restServer http.Handler

func TestApplication(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "ApiServer Suite")
}
