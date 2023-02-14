package gen_sdk_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGenSdk(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GenSdk Suite")
}
