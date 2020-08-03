package component

import (
	"fmt"

	rudr "github.com/cloud-native-application/rudrx/pkg/test"
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
			output, err := rudr.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("SUCCEED"))
		})
	})

	ginkgo.Context("delete", func() {
		ginkgo.It("should print successful deletion information", func() {
			cli := fmt.Sprintf("rudr delete %s", applicationName)
			output, err := rudr.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("DELETE SUCCEED"))
		})
	})
})
