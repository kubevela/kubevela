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

package e2e

import (
	"fmt"

	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/references/apis"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	registryConfigs = []apis.RegistryConfig{
		{
			Name:  "e2e-oss-registry",
			URL:   "oss://registry.e2e.net",
			Token: "",
		},
		{
			Name:  "e2e-github-registry",
			URL:   "https://github.com/oam-dev/catalog/tree/master/traits",
			Token: "",
		},
	}
)

// TODO: change this into a mock UT to avoid remote call.

var _ = Describe("test registry and trait/comp command", func() {
	Context("registry", func() {
		It("add and remove registry config", func() {
			for _, config := range registryConfigs {
				cli := fmt.Sprintf("vela registry config %s %s", config.Name, config.URL)
				output, err := e2e.Exec(cli)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(ContainSubstring(fmt.Sprintf("Successfully configured registry %s", config.Name)))
			}
		})

		It("list registry config", func() {
			cli := "vela registry ls"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("NAME"))
			Expect(output).To(ContainSubstring("URL"))
			for _, config := range registryConfigs {
				Expect(output).To(ContainSubstring(config.Name))
				Expect(output).To(ContainSubstring(config.URL))
			}
		})

		It("remove registry config", func() {
			for _, config := range registryConfigs {
				cli := fmt.Sprintf("vela registry remove %s", config.Name)
				output, err := e2e.Exec(cli)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(ContainSubstring(fmt.Sprintf("Successfully remove registry %s", config.Name)))
			}

		})
	})

	Context("list and install trait from registry", func() {
		It("list trait from cluster", func() {
			cli := "vela trait"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("NAME"))
			Expect(output).To(ContainSubstring("APPLIES-TO"))
			Expect(output).To(ContainSubstring("pvc"))
			Expect(output).To(ContainSubstring("[deployments.apps]"))
		})
		It("list trait from default registry", func() {
			cli := "vela trait --discover"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Showing trait definition from registry: default"))
			Expect(output).To(ContainSubstring("NAME"))
			Expect(output).To(ContainSubstring("APPLIES-TO"))
			Expect(output).To(ContainSubstring("STATUS"))
			Expect(output).To(ContainSubstring("autoscale"))
			Expect(output).To(ContainSubstring("[deployments.apps]"))
		})

		It("test list trait in raw url", func() {
			cli := "vela trait --discover --url=oss://registry.kubevela.net"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(SatisfyAll(ContainSubstring("Showing trait definition from url"), ContainSubstring("oss://registry.kubevela.net")))
		})

	})

	Context("test list component definition", func() {
		It("test list installed component definition", func() {
			cli := "vela comp"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("NAME"))
			Expect(output).To(ContainSubstring("DEFINITION"))
			Expect(output).To(ContainSubstring("raw"))
			Expect(output).To(ContainSubstring("deployments.apps"))
		})
		It("test list with label", func() {
			cli := "vela comp --label type=terraform"
			output, err := e2e.Exec(cli)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(ContainSubstring("raw"))
			Expect(output).To(ContainSubstring("alibaba-ack"))
		})
	})
})
