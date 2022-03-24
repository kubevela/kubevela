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
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test project rest api", func() {
	var (
		projectName1 string
	)
	BeforeEach(func() {
		projectName1 = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	})
	It("Test create project", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateProjectRequest{
			Name:        projectName1,
			Description: "KubeVela Project",
		}
		res := post("/projects", req)
		var projectBase apisv1.ProjectBase
		Expect(decodeResponseBody(res, &projectBase)).Should(Succeed())
		Expect(cmp.Diff(projectBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(projectBase.Description, req.Description)).Should(BeEmpty())
	})

	It("Test list project", func() {
		defer GinkgoRecover()
		res := get("/projects")
		var projects apisv1.ListProjectResponse
		Expect(decodeResponseBody(res, &projects)).Should(Succeed())
	})
})
