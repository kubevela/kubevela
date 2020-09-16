package e2e

import (
	"fmt"

	"github.com/oam-dev/kubevela/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	envName                       = "env-workload"
	applicationName               = "app-workload-basic"
	notEnoughFlagsApplicationName = "app-workload-basic"
)

var _ = ginkgo.Describe("Workload", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvSetContext("env set", envName)
	e2e.WorkloadRunContext("run", fmt.Sprintf("vela comp run -t webservice %s -p 80 --image nginx:1.9.4", applicationName))

	ginkgo.Context("run without enough flags", func() {
		ginkgo.It("should throw error message: some flags are NOT set", func() {
			cli := fmt.Sprintf("vela comp run -t webservice %s -p 80", notEnoughFlagsApplicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("required flag(s) \"image\" not set"))
		})
	})

	e2e.WorkloadDeleteContext("delete", applicationName)
})
