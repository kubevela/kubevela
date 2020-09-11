package e2e

import (
	"github.com/oam-dev/kubevela/e2e"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	envName  = "env-hello"
	envName2 = "env-world"
)

var _ = ginkgo.Describe("Env", func() {
	e2e.RefreshContext("refresh")
	e2e.EnvInitContext("env init", envName)
	e2e.EnvInitContext("env init another one", envName2)
	e2e.EnvShowContext("env show", envName)
	e2e.EnvSetContext("env sw", envName)

	ginkgo.Context("env list", func() {
		ginkgo.It("should list all envs", func() {
			output, err := e2e.Exec("vela env ls")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
			gomega.Expect(output).To(gomega.ContainSubstring(envName))
			gomega.Expect(output).To(gomega.ContainSubstring(envName2))
		})
	})

	e2e.EnvDeleteContext("env delete", envName2)
	e2e.EnvDeleteCurrentUsingContext("env delete currently using one", envName)
	// TODO(zzxwill) Delete an env which does not exist
})
