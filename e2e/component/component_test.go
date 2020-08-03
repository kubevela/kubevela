package e2e

import (
	"fmt"
	"github.com/cloud-native-application/rudrx/e2e"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	applicationName = "app-testworkloadrun-basic"
)

var _ = ginkgo.Describe("Component", func() {
	ginkgo.Context("run", func() {
		ginkgo.It("should print successful creation information", func() {
			cli := fmt.Sprintf("rudr containerized:run %s -p 80 --image nginx:1.9.4", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("SUCCEED"))
		})
	})

	ginkgo.Context("delete", func() {
		ginkgo.It("should print successful deletion information", func() {
			cli := fmt.Sprintf("rudr delete %s", applicationName)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("DELETE SUCCEED"))
		})
	})
})
