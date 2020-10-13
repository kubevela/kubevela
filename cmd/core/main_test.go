package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	testdir      = "testdir"
	testTimeout  = 2 * time.Second
	testInterval = 1 * time.Second
)

func TestGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "test main")
}

var _ = Describe("test waitSecretVolume", func() {
	BeforeEach(func() {
		err := os.MkdirAll(testdir, 0755)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		os.RemoveAll(testdir)
	})

	When("dir not exist or empty", func() {
		It("return timeout error", func() {
			err := waitWebhookSecretVolume(testdir, testTimeout, testInterval)
			Expect(err).To(HaveOccurred())
			By("remove dir")
			os.RemoveAll(testdir)
			err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
			Expect(err).To(HaveOccurred())
		})
	})

	When("dir contains empty file", func() {
		It("return timeout error", func() {
			By("add empty file")
			_, err := os.Create(testdir + "/emptyFile")
			Expect(err).NotTo(HaveOccurred())
			err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
			Expect(err).To(HaveOccurred())
		})
	})

	When("files in dir are not empty", func() {
		It("return nil", func() {
			By("add non-empty file")
			_, err := os.Create(testdir + "/file")
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(testdir+"/file", []byte("test"), os.ModeAppend)
			Expect(err).NotTo(HaveOccurred())
			err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
