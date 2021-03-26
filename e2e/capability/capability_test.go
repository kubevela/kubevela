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

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/e2e"
	"github.com/oam-dev/kubevela/references/apiserver/apis"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	capabilityCenterBasic = apis.CapabilityCenterMeta{
		Name: "capability-center-e2e-basic",
		URL:  "https://github.com/oam-dev/kubevela/tree/master/pkg/plugins/testdata",
	}

	websvcCapability = types.Capability{
		Name: "webservice.testapps",
		Type: types.TypeWorkload,
	}

	scaleCapability = types.Capability{
		Name: "scaler",
		Type: types.TypeTrait,
	}

	routeCapability = types.Capability{
		Name: "routes.test",
		Type: types.TypeTrait,
	}

	ingressCapability = types.Capability{
		Name: "ingress.test",
		Type: types.TypeTrait,
	}
)

// TODO: chagne this into a mock UT to avoid remote call.

var _ = ginkgo.Describe("Capability", func() {
	ginkgo.Context("capability center", func() {
		ginkgo.It("add a capability center", func() {
			cli := fmt.Sprintf("vela cap center config %s %s", capabilityCenterBasic.Name, capabilityCenterBasic.URL)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput1 := fmt.Sprintf("Successfully configured capability center %s and sync from remote", capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput1))
		})

		ginkgo.It("list capability centers", func() {
			cli := "vela cap center ls"
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("ADDRESS"))
			gomega.Expect(output).To(gomega.ContainSubstring(capabilityCenterBasic.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(capabilityCenterBasic.URL))
		})
	})

	ginkgo.Context("capability", func() {
		ginkgo.It("install a workload capability to cluster", func() {
			cli := fmt.Sprintf("vela cap install %s/%s", capabilityCenterBasic.Name, websvcCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr1 := fmt.Sprintf("Installing %s capability", websvcCapability.Type)
			expectedSubStr2 := fmt.Sprintf("Successfully installed capability %s from %s", websvcCapability.Name, capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr1))
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr2))
		})

		ginkgo.It("install a trait capability to cluster", func() {
			cli := fmt.Sprintf("vela cap install %s/%s", capabilityCenterBasic.Name, scaleCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr1 := fmt.Sprintf("Installing %s capability", scaleCapability.Type)
			expectedSubStr2 := fmt.Sprintf("Successfully installed capability %s from %s", scaleCapability.Name, capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr1))
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr2))
		})

		ginkgo.It("install a trait capability without definition reference to cluster", func() {
			cli := fmt.Sprintf("vela cap install %s/%s", capabilityCenterBasic.Name, ingressCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr1 := fmt.Sprintf("Installing %s capability", ingressCapability.Type)
			expectedSubStr2 := fmt.Sprintf("Successfully installed capability %s from %s", ingressCapability.Name, capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr1))
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr2))
		})

		ginkgo.It("list all capabilities", func() {
			cli := fmt.Sprintf("vela cap ls %s", capabilityCenterBasic.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("CENTER"))
			gomega.Expect(output).To(gomega.ContainSubstring(websvcCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(ingressCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(scaleCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring(routeCapability.Name))
			gomega.Expect(output).To(gomega.ContainSubstring("installed"))
		})

		ginkgo.It("uninstall a workload capability from cluster", func() {
			cli := fmt.Sprintf("vela cap uninstall %s", websvcCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr := fmt.Sprintf("Successfully uninstalled capability %s", websvcCapability.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr))
		})

		ginkgo.It("uninstall a trait capability from cluster", func() {
			cli := fmt.Sprintf("vela cap uninstall %s", ingressCapability.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedSubStr := fmt.Sprintf("Successfully uninstalled capability %s", ingressCapability.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedSubStr))

			// unstall other installed test capability
			cli = fmt.Sprintf("vela cap uninstall %s", scaleCapability.Name)
			_, err = e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})

		ginkgo.It("delete a capability center", func() {
			cli := fmt.Sprintf("vela cap center remove %s", capabilityCenterBasic.Name)
			output, err := e2e.Exec(cli)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			expectedOutput := fmt.Sprintf("%s capability center removed successfully", capabilityCenterBasic.Name)
			gomega.Expect(output).To(gomega.ContainSubstring(expectedOutput))
		})
	})
})
