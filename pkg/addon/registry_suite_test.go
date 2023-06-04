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

package addon

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test Registry ListAddonInfo()", func() {
	Context("helm repo", func() {
		registry := Registry{
			Name: "helm-repo",
			Helm: &HelmSource{URL: "http://127.0.0.1:18083/multi"},
		}
		It("return addon info", func() {
			addons, err := registry.ListAddonInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(addons).To(HaveLen(2))
			Expect(addons).To(HaveKey("fluxcd"))
			Expect(addons["fluxcd"].AvailableVersions).To(Equal([]string{"2.0.0", "1.0.0"}))
		})
	})
	Context("local repo", func() {
		registry := Registry{
			Name: LocalAddonRegistryName,
		}
		It("return empty map", func() {
			addons, err := registry.ListAddonInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(addons).To(HaveLen(0))
		})
	})
})
