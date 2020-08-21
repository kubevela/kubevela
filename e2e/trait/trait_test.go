package e2e

import (
	"fmt"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var (
	envName         = "env-trait"
	applicationName = "app-trait-basic"
	traitAlias      = "scale"
)

var _ = ginkgo.Describe("Trait", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSwitchContext("env switch", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("vela containerized:run %s -p 80 --image nginx:1.9.4", applicationName))

	e2e.TraitManualScalerAttachContext("vela attach trait", traitAlias, applicationName)

	//ginkgo.Context("vela detach trait", func() {
	//	ginkgo.It("should print successful detached information", func() {
	//		cli := fmt.Sprintf("vela ManualScaler %s --detach", applicationName)
	//		output, err := e2e.Exec(cli)
	//		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	//		gomega.Expect(output).To(gomega.ContainSubstring("Applying trait for app"))
	//		gomega.Expect(output).To(gomega.ContainSubstring("Succeeded"))
	//	})
	//})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
