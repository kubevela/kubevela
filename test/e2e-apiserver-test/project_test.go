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

var _ = Describe("Test project rest api", func() {
	It("Test create project", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateProjectRequest{
			Name:        "dev-team",
			Description: "KubeVela Project",
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/projects", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var projectBase apisv1.ProjectBase
		err = json.NewDecoder(res.Body).Decode(&projectBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(projectBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(projectBase.Description, req.Description)).Should(BeEmpty())
	})

	It("Test list project", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/projects")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var projects apisv1.ListProjectResponse
		err = json.NewDecoder(res.Body).Decode(&projects)
		Expect(err).ShouldNot(HaveOccurred())
	})
})
