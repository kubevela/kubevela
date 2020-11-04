package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/e2e"
)

var (
	envName                   = "env-trait"
	applicationName           = "app-trait-basic"
	applicationNotExistedName = "app-trait-basic-NOT-EXISTED"
	traitAlias                = "scaler"
	serviceNameNotExisting    = "svc-not-existing"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.TraitCapabilityListContext()
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.WorkloadRunContext("deploy", fmt.Sprintf("vela svc deploy -t webservice %s -p 80 --image nginx:1.9.4", applicationName))

	e2e.TraitManualScalerAttachContext("vela attach trait", traitAlias, applicationName)

	// Trait
	ginkgo.Context("vela attach trait to a not existing app", func() {
		ginkgo.It("should alert app not exist", func() {
			cli := fmt.Sprintf("vela %s %s", traitAlias, applicationNotExistedName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("the application " + applicationNotExistedName + " doesn't exist in current env " + envName))
		})
	})

	ginkgo.Context("vela attach trait to a not existing service", func() {
		ginkgo.It("should alert service not exist", func() {
			cli := fmt.Sprintf("vela %s %s --svc %s", traitAlias, applicationName, serviceNameNotExisting)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("the service " + serviceNameNotExisting + " doesn't exist in the application " + applicationName))
		})
	})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
