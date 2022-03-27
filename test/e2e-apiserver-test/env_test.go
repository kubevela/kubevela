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
	"github.com/oam-dev/kubevela/pkg/multicluster"
)

var _ = Describe("Test env rest api", func() {
	var (
		testtarget1, testenv1, testtarget2 string
	)
	BeforeEach(func() {
		testtarget1 = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
		testenv1 = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
		testtarget2 = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
	})

	It("Test create, get, delete env with normal format", func() {
		defer GinkgoRecover()

		By("create a target for preparation")
		var reqt = apisv1.CreateTargetRequest{
			Name:        testtarget1,
			Alias:       "my-target-for-env1",
			Description: "KubeVela Target",
			Project:     "my-pro",
			Cluster:     &apisv1.ClusterTarget{ClusterName: multicluster.ClusterLocalName, Namespace: testtarget1},
		}
		var tgBase apisv1.TargetBase
		resp := post("/targets", reqt)
		Expect(decodeResponseBody(resp, &tgBase)).Should(Succeed())

		By("create the first env")
		var req = apisv1.CreateEnvRequest{
			Name:        testenv1,
			Alias:       "my=test!",
			Project:     "my-pro",
			Description: "KubeVela Env",
			Namespace:   testenv1,
			Targets:     []string{testtarget1},
		}
		var envBase apisv1.Env
		resp = post("/envs", req)
		Expect(decodeResponseBody(resp, &envBase)).Should(Succeed())
		Expect(cmp.Diff(envBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(envBase.Description, req.Description)).Should(BeEmpty())

		By("get the first env")
		var envs apisv1.ListEnvResponse
		resp = get("/envs")
		Expect(decodeResponseBody(resp, &envs)).Should(Succeed())
		Expect(len(envs.Envs) >= 1).Should(BeTrue())
		var found bool
		for _, ev := range envs.Envs {
			if ev.Name != req.Name {
				found = true
				continue
			}
			Expect(ev.Alias).Should(BeEquivalentTo(req.Alias))
			Expect(ev.Project.Name).Should(BeEquivalentTo(req.Project))
			Expect(ev.Description).Should(BeEquivalentTo(req.Description))
			Expect(ev.Namespace).Should(BeEquivalentTo(req.Namespace))
			Expect(ev.Targets).Should(BeEquivalentTo([]apisv1.NameAlias{{Name: testtarget1, Alias: "my-target-for-env1"}}))
		}
		Expect(found).Should(BeTrue())

		By("delete the first env")
		resp = delete("/envs/" + testenv1)
		Expect(decodeResponseBody(resp, nil)).Should(Succeed())

	})

	It("Test crate, update, list env", func() {
		defer GinkgoRecover()

		By("create a target for preparation")
		var reqt = apisv1.CreateTargetRequest{
			Name:        testtarget1,
			Alias:       "my-target-for-env2",
			Description: "KubeVela Target",
			Project:     "my-pro",
			Cluster:     &apisv1.ClusterTarget{ClusterName: multicluster.ClusterLocalName, Namespace: testtarget1},
		}
		var tgBase apisv1.TargetBase
		resp := post("/targets", reqt)
		Expect(decodeResponseBody(resp, &tgBase)).Should(Succeed())
		reqt = apisv1.CreateTargetRequest{
			Name:        testtarget2,
			Alias:       "my-target-for-env3",
			Description: "KubeVela Target",
			Project:     "my-pro",
			Cluster:     &apisv1.ClusterTarget{ClusterName: multicluster.ClusterLocalName, Namespace: testtarget2},
		}
		resp = post("/targets", reqt)
		Expect(decodeResponseBody(resp, &tgBase)).Should(Succeed())

		By("create  env for update")
		var req = apisv1.CreateEnvRequest{
			Name:        testenv1,
			Alias:       "my=test!",
			Project:     "my-pro",
			Namespace:   testenv1,
			Description: "KubeVela Env",
			Targets:     []string{testtarget1},
		}
		var envBase apisv1.Env
		resp = post("/envs", req)
		Expect(decodeResponseBody(resp, &envBase)).Should(Succeed())
		Expect(cmp.Diff(envBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(envBase.Description, req.Description)).Should(BeEmpty())

		By("update the env")
		upreq := apisv1.UpdateEnvRequest{
			Alias:       "my=test3",
			Description: "KubeVela Env2",
			Targets:     []string{testtarget2},
		}
		resp = put("/envs/"+testenv1, upreq)
		Expect(decodeResponseBody(resp, nil)).Should(Succeed())

		By("get the env")
		var envs apisv1.ListEnvResponse
		resp = get("/envs")
		Expect(decodeResponseBody(resp, &envs)).Should(Succeed())
		Expect(len(envs.Envs) >= 1).Should(BeTrue())
		var found bool
		for _, ev := range envs.Envs {
			if ev.Name != req.Name {
				found = true
				continue
			}
			Expect(ev.Alias).Should(BeEquivalentTo("my=test3"))
			Expect(ev.Project.Name).Should(BeEquivalentTo(req.Project))
			Expect(ev.Description).Should(BeEquivalentTo("KubeVela Env2"))
			Expect(ev.Namespace).Should(BeEquivalentTo(req.Namespace))
			Expect(ev.Targets).Should(BeEquivalentTo([]apisv1.NameAlias{{Name: testtarget2, Alias: "my-target-for-env3"}}))
		}
		Expect(found).Should(BeTrue())

	})
})
