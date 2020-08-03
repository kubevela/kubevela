package e2e

import (
	"fmt"

	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	envName  = "env-hello"
	envName2 = "env-world"
)

var _ = ginkgo.Describe("Env", func() {
	ginkgo.Context("env init", func() {
		ginkgo.It("should print env initiation successful message", func() {
			cli := fmt.Sprintf("rudr env:init %s --namespace %s", envName, envName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("Create env succeed, current env is %s", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})

	ginkgo.Context("env init another one", func() {
		ginkgo.It("should print env initiation successful message", func() {
			cli := fmt.Sprintf("rudr env:init %s --namespace %s", envName2, envName2)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("Create env succeed, current env is %s", envName2)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})

	ginkgo.Context("env show", func() {
		ginkgo.It("should show detailed env message", func() {
			cli := fmt.Sprintf("rudr env %s", envName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("%s\t%s", envName, envName)
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
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

	ginkgo.Context("env list", func() {
		ginkgo.It("should list all envs", func() {
			output, err := e2e.Exec("rudr env")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
			gomega.Expect(output).To(gomega.ContainSubstring(envName))
			gomega.Expect(output).To(gomega.ContainSubstring(envName2))
		})
	})

	ginkgo.Context("env delete", func() {
		ginkgo.It("should delete all envs", func() {
			cli := fmt.Sprintf("rudr env:delete %s", envName2)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("%s deleted", envName2)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})

	ginkgo.Context("env delete currently using one", func() {
		ginkgo.It("should delete all envs", func() {
			cli := fmt.Sprintf("rudr env:delete %s", envName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("Error: you can't delete current using env %s", envName)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})
})
