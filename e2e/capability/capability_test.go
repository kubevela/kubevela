package e2e

import (
	"fmt"

	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/pkg/server/apis"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	capabilityCenterBasic = apis.CapabilityCenterMeta{
		Name: "capability-center-e2e-basic",
		URL:  "https://github.com/oam-dev/kubevela/tree/master/pkg/plugins/testdata",
	}

	scaleCapability = types.Capability{
		Name: "scale",
		Type: types.TypeTrait,
	}

	routeCapability = types.Capability{
		Name: "route",
		Type: types.TypeTrait,
	}
)

var _ = ginkgo.Describe("Capability", func() {
	ginkgo.Context("capability center", func() {
		ginkgo.It("add a capability center", func() {
			cli := fmt.Sprintf("vela cap center config %s %s", capabilityCenterBasic.Name, capabilityCenterBasic.URL)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput1 := fmt.Sprintf("Successfully configured capability center: %s, start to sync from remote", capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput1))
			gomega.Expect(output).To(gomega.ContainSubstring("sync finished"))
		})

		ginkgo.It("list capability centers", func() {
			cli := "vela cap center ls"
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("ADDRESS"))
			gomega.Expect(output).To(gomega.ContainSubstring(capabilityCenterBasic.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(capabilityCenterBasic.URL))
		})
	})

	ginkgo.Context("capability", func() {
		ginkgo.It("install a capability to cluster", func() {
			cli := fmt.Sprintf("vela cap add %s/%s", capabilityCenterBasic.Name, scaleCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr1 := fmt.Sprintf("Installing %s capability", scaleCapability.Type)
			expectedSubStr2 := fmt.Sprintf("Successfully installed capability %s from %s", scaleCapability.Name, capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr1))
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr2))
		})

		ginkgo.It("list all capabilities", func() {
			cli := fmt.Sprintf("vela cap ls %s", capabilityCenterBasic.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("CENTER"))
			gomega.Expect(output).To(gomega.ContainSubstring(scaleCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(routeCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring("installed"))
		})

		ginkgo.It("delete a capability center", func() {
			cli := fmt.Sprintf("vela cap center remove %s", capabilityCenterBasic.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("%s capability center removed successfully", capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})
})
