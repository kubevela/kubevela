/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/oam-dev/kubevela/cmd/core/app/options"
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

var _ = Describe("test SetupLogging", func() {
	var (
		opts *options.CoreOptions
		cmd  *cobra.Command
	)

	BeforeEach(func() {
		opts = options.NewCoreOptions()
		cmd = &cobra.Command{}
		cmd.Flags().String("v", "0", "log level")
	})

	Context("when LogDebug is enabled", func() {
		It("should set verbosity flag to debug level", func() {
			opts.LogDebug = true
			err := SetupLogging(opts, cmd)
			Expect(err).NotTo(HaveOccurred())

			vFlag := cmd.Flags().Lookup("v")
			Expect(vFlag).NotTo(BeNil())
			Expect(vFlag.Value.String()).To(Equal("1"))
		})
	})

	Context("when LogDebug is disabled", func() {
		It("should not modify verbosity flag", func() {
			opts.LogDebug = false
			err := SetupLogging(opts, cmd)
			Expect(err).NotTo(HaveOccurred())

			vFlag := cmd.Flags().Lookup("v")
			Expect(vFlag).NotTo(BeNil())
			Expect(vFlag.Value.String()).To(Equal("0")) // Should remain default
		})
	})

	Context("when LogFilePath is set", func() {
		It("should complete without error and log warning", func() {
			opts.LogFilePath = "/path/to/log"
			err := SetupLogging(opts, cmd)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("when v flag does not exist", func() {
		It("should handle missing flag gracefully", func() {
			cmdWithoutV := &cobra.Command{}
			opts.LogDebug = true
			err := SetupLogging(opts, cmdWithoutV)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

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
			err = os.WriteFile(testdir+"/file", []byte("test"), 0644)
			Expect(err).NotTo(HaveOccurred())
			err = waitWebhookSecretVolume(testdir, testTimeout, testInterval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

})
