package cli

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("parse Options", func() {
	var opt Option
	BeforeEach(func() {
		cli = nil
		cfg = nil
		dm = nil
		pd = nil
		opt = Option{}
	})

	It("parse `vela status`", func() {
		command := []string{"vela", "status"}
		opt = parseOption(command)
		Expect(opt.Must).Should(Equal([]Tool{DynamicClient}))
	})

	It("parse `vela def gen-api`", func() {
		command := []string{"vela", "def", "gen-api"}
		parseOption(command)
		Expect(opt.Must).Should(Equal([]Tool{}))
	})

})
