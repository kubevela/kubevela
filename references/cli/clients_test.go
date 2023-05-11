package cli

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("parse Options", func() {
	BeforeEach(func() {
		cli = nil
		cfg = nil
		dm = nil
		pd = nil
	})
	command := []string{"vela", "status"}
	InitClients(command)
	Expect(cli).ShouldNot(BeNil())
})

var _ = Describe("init clients", func() {

})
