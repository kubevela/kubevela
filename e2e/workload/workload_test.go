package e2e

import (
	"github.com/onsi/ginkgo"

	"github.com/oam-dev/kubevela/e2e"
)

var _ = ginkgo.Describe("Workload", func() {
	e2e.WorkloadCapabilityListContext()
})
