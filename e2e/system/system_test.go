package e2e

import (
	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var _ = ginkgo.Describe("Application", func() {
	e2e.SystemInitContext("system init")
})
