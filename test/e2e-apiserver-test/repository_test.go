/*
Copyright 2022 The KubeVela Authors.

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

package e2e_apiserver_test

import (
	"io"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Helm rest api test", func() {

	Describe("helm repo api test", func() {
		It("test fetching chart values in OCI registry", func() {
			resp := getWithQuery("/repository/chart/values", map[string]string{
				"repoUrl":  "oci://ghcr.io",
				"chart":    "stefanprodan/charts/podinfo",
				"repoType": "oci",
				"version":  "6.1.0",
			})
			defer resp.Body.Close()
			values, err := io.ReadAll(resp.Body)
			Expect(err).Should(BeNil())
			Expect(len(values)).ShouldNot(BeEquivalentTo(0))
		})
	})
})
