package e2e

import (
	"fmt"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	//TODO(zzxwill) Need to change env name after [issue#82](https://github.com/cloud-native-application/RudrX/issues/82) is fixed.
	envName         = "default"
	applicationName = "app-testworkloadrun-basic"
)

var _ = ginkgo.Describe("Workload", func() {
	ginkgo.Context("env init", func() {
		ginkgo.It("should print env initiation successful message", func() {
			cli := fmt.Sprintf("rudr env:init %s --namespace %s", envName, envName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("Create env succeed, current env is %s", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})

	ginkgo.Context("env sw", func() {
		ginkgo.It("should show env switch message", func() {
			cli := fmt.Sprintf("rudr env:sw %s", envName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("Switch env succeed, current env is %s", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})

	ginkgo.Context("run", func() {
		ginkgo.It("should print successful creation information", func() {
			cli := fmt.Sprintf("rudr containerized:run %s -p 80 --image nginx:1.9.4", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("SUCCEED"))
		})
	})

	e2e.WorkloadDeleteContext(applicationName)
})
