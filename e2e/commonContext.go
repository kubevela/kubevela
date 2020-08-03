package e2e

import (
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	WorkloadDeleteContext = func(applicationName string) bool {
		return ginkgo.Context("delete", func() {
			ginkgo.It("should print successful deletion information", func() {
				cli := fmt.Sprintf("rudr delete %s", applicationName)
				output, err := Exec(cli)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(output).To(gomega.ContainSubstring("DELETE SUCCEED"))
			})
		})
	}
)
