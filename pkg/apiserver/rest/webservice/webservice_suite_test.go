package webservice_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWebservice(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Webservice Suite")
}
