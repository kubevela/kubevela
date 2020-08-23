package e2e

import (
	"fmt"

	"github.com/onsi/gomega"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var (
	envName                   = "env-trait"
	applicationName           = "app-trait-basic"
	applicationNotExistedName = "app-trait-basic-NOT-EXISTED"
	traitAlias                = "scale"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSwitchContext("env switch", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("vela comp run -t containerized %s -p 80 --image nginx:1.9.4", applicationName))

	e2e.TraitManualScalerAttachContext("vela attach trait", traitAlias, applicationName)

	// Trait
	ginkgo.Context("vela attach trait to a not existed app", func() {
		ginkgo.It("should print successful attached information", func() {
			cli := fmt.Sprintf("vela %s %s", traitAlias, applicationNotExistedName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Error: " + applicationNotExistedName + " not exist"))
		})
	})

	ginkgo.Context("vela detach trait", func() {
		ginkgo.It("should print successful detached information", func() {
			cli := fmt.Sprintf("vela %s:detach %s", traitAlias, applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring(fmt.Sprintf("Detaching %s from %s", traitAlias, applicationName)))
			gomega.Expect(output).To(gomega.ContainSubstring("Succeeded!"))
		})
	})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
