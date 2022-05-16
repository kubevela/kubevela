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

package e2e_multicluster_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test multicluster Auth commands", func() {

	Context("Test vela auth commands", func() {

		It("Test vela create kubeconfig for given user", func() {
			outputs, err := execCommand("create", "kubeconfig", "--user", "kubevela", "--group", "kubevela:dev", "--group", "kubevela:test")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("Certificate signing request kubevela approved"))
		})

		It("Test vela create kubeconfig for serviceaccount", func() {
			outputs, err := execCommand("create", "kubeconfig", "--serviceaccount", "default", "-n", "vela-system")
			Expect(err).Should(Succeed())
			Expect(outputs).Should(ContainSubstring("ServiceAccount vela-system/default found."))
		})

	})

})
