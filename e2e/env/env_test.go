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
	"github.com/oam-dev/kubevela/e2e"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
)

var (
	envName  = "env-hello"
	envName2 = "env-world"
)

var _ = ginkgo.Describe("Env", func() {
	e2e.EnvInitWithNamespaceOptionContext("env init env-hello --namespace heelo", envName, "heelo")
	e2e.EnvInitWithNamespaceOptionContext("env init another one --namespace heelo2", envName2, "heelo2")
	e2e.EnvShowContext("env show", envName)
	e2e.EnvSetContext("env set", envName)

	ginkgo.Context("env list", func() {
		ginkgo.It("should list all envs", func() {
			output, err := e2e.Exec("vela env ls")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(output).To(gomega.ContainSubstring("NAME"))
			gomega.Expect(output).To(gomega.ContainSubstring("NAMESPACE"))
			gomega.Expect(output).To(gomega.ContainSubstring(envName))
			gomega.Expect(output).To(gomega.ContainSubstring(envName2))
		})
	})

	e2e.EnvDeleteContext("env delete", envName2)
})
