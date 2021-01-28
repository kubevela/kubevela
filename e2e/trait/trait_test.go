package e2e

import (
	"github.com/onsi/ginkgo"

	"github.com/oam-dev/kubevela/e2e"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.TraitCapabilityListContext()
})
