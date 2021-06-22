package utils

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("utils", func() {
	Context("GetEnabledCapabilities", func() {
		It("disable all", func() {
			disableCaps := "all"
			err := CheckDisabledCapabilities(disableCaps)
			Expect(err).NotTo(HaveOccurred())
		})
		It("disable none", func() {
			disableCaps := ""
			err := CheckDisabledCapabilities(disableCaps)
			Expect(err).NotTo(HaveOccurred())
		})
		It("disable some capabilities", func() {
			disableCaps := "application"
			err := CheckDisabledCapabilities(disableCaps)
			Expect(err).NotTo(HaveOccurred())
		})
		It("disable some bad capabilities", func() {
			disableCaps := "abc,def"
			err := CheckDisabledCapabilities(disableCaps)
			Expect(err).To(HaveOccurred())
		})
	})
})
