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

package e2e_apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test namespace rest api", func() {
	It("Test create namespace", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateNamespaceRequest{
			Name:        "dev-team",
			Description: "开发环境租户",
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/namespaces", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var namespaceBase apisv1.NamespaceBase
		err = json.NewDecoder(res.Body).Decode(&namespaceBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(namespaceBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(namespaceBase.Description, req.Description)).Should(BeEmpty())
	})

	It("Test list namespace", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/namespaces")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var namespaces apisv1.ListNamespaceResponse
		err = json.NewDecoder(res.Body).Decode(&namespaces)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
