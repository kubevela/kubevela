package utils

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test cache utils", func() {
	It("should return false for IsExpired()", func() {
		c := NewMemoryCache("test", 10*time.Hour)
		Expect(c.IsExpired()).Should(BeFalse())
	})
})
