package e2e

import (
	"fmt"

	"github.com/onsi/gomega"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
)

var (
	//TODO(zzxwill) Need to change env name after [issue#82](https://github.com/cloud-native-application/RudrX/issues/82) is fixed.
	envName         = "default"
	applicationName = "app-trait-basic"
)

var _ = ginkgo.Describe("Env", func() {
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSwitchContext("env switch", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("rudr containerized:run %s -p 80 --image nginx:1.9.4", applicationName))

	ginkgo.Context("rudr attach trait", func() {
		ginkgo.It("should print successful attached information", func() {
			cli := fmt.Sprintf("rudr ManualScaler %s --replicaCount 4", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("Applying trait for app"))
			gomega.Expect(output).To(gomega.ContainSubstring("Succeeded"))
		})
	})

	//ginkgo.Context("rudr detach trait", func() {
	//	ginkgo.It("should print successful detached information", func() {
	//		cli := fmt.Sprintf("rudr ManualScaler %s --detach", applicationName)
	//		output, err := e2e.Exec(cli)
	//		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	//		gomega.Expect(output).To(gomega.ContainSubstring("Applying trait for app"))
	//		gomega.Expect(output).To(gomega.ContainSubstring("Succeeded"))
	//	})
	//})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
