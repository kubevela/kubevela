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

package e2e_apiserver

import (
	"encoding/json"
	"net/http"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test env rest api", func() {
	It("Test create, get, delete env with normal format", func() {
		defer GinkgoRecover()

		By("create a target for preparation")
		var reqt = apisv1.CreateTargetRequest{
			Name:        "t1",
			Alias:       "my-target-for-env1",
			Description: "KubeVela Target",
		}
		var tgBase apisv1.TargetBase
		err := HttpRequest(reqt, http.MethodPost, "/targets", &tgBase)
		Expect(err).ShouldNot(HaveOccurred())

		By("create the first env")
		var req = apisv1.CreateEnvRequest{
			Name:        "dev-env",
			Alias:       "my=test!",
			Project:     "my-pro",
			Description: "KubeVela Env",
			Namespace:   "my-name",
			Targets:     []string{"t1"},
		}
		var envBase apisv1.Env
		err = HttpRequest(req, http.MethodPost, "/envs", &envBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(envBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(envBase.Description, req.Description)).Should(BeEmpty())

		By("get the first env")
		var envs apisv1.ListEnvResponse
		err = HttpRequest(nil, http.MethodGet, "/envs", &envs)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(len(envs.Envs) >= 1).Should(BeTrue())
		var found bool
		for _, ev := range envs.Envs {
			if ev.Name != req.Name {
				found = true
				continue
			}
			Expect(ev.Alias).Should(BeEquivalentTo(req.Alias))
			Expect(ev.Project).Should(BeEquivalentTo(req.Project))
			Expect(ev.Description).Should(BeEquivalentTo(req.Description))
			Expect(ev.Namespace).Should(BeEquivalentTo(req.Namespace))
			Expect(ev.Targets).Should(BeEquivalentTo([]apisv1.NameAlias{{Name: "t1", Alias: "my-target-for-env1"}}))
		}
		Expect(found).Should(BeTrue())
	})

	It("Test crate, update, list env", func() {
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
