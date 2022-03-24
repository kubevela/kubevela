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
	"context"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/model"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var appName = "app-e2e"
var appProject = "test-app-project"

var _ = Describe("Test application rest api", func() {
	It("Test create app", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationRequest{
			Name:        appName,
			Project:     appProject,
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			EnvBinding:  []*apisv1.EnvBinding{{Name: "dev-env"}},
			Component: &apisv1.CreateComponentRequest{
				Name:          "webservice",
				ComponentType: "webservice",
				Properties:    "{\"image\":\"nginx\"}",
			},
		}
		res := post("/applications", req)
		var appBase apisv1.ApplicationBase
		Expect(decodeResponseBody(res, &appBase)).Should(Succeed())
		Expect(cmp.Diff(appBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Labels["test"], req.Labels["test"])).Should(BeEmpty())
	})

	It("Test list components", func() {
		defer GinkgoRecover()
		res := get("/applications/" + appName + "/components")
		var components apisv1.ComponentListResponse
		Expect(decodeResponseBody(res, &components)).Should(Succeed())
		Expect(cmp.Diff(len(components.Components), 1)).Should(BeEmpty())
	})

	It("Test detail application", func() {
		defer GinkgoRecover()
		res := get("/applications/" + appName)
		var detail apisv1.DetailApplicationResponse
		Expect(decodeResponseBody(res, &detail)).Should(Succeed())
		Expect(cmp.Diff(len(detail.Policies), 0)).Should(BeEmpty())
	})

	It("Test deploy application", func() {
		defer GinkgoRecover()
		var targetName = testNSprefix + strconv.FormatInt(time.Now().UnixNano(), 10)
		var envName = "dev"
		// create target
		var createTarget = apisv1.CreateTargetRequest{
			Name: targetName,
			Cluster: &apisv1.ClusterTarget{
				ClusterName: "local",
				Namespace:   targetName,
			},
		}
		res := post("/targets", createTarget)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())

		// create env
		var createEnvReq = apisv1.CreateEnvRequest{
			Name:    envName,
			Targets: []string{targetName},
		}
		res = post("/envs", createEnvReq)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())

		// create envbinding
		var createEnvbindingReq = apisv1.CreateApplicationEnvbindingRequest{
			EnvBinding: apisv1.EnvBinding{
				Name: envName,
			},
		}
		res = post("/applications/"+appName+"/envs", createEnvbindingReq)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())

		// deploy app
		var req = apisv1.ApplicationDeployRequest{
			Note:         "test apply",
			TriggerType:  "web",
			WorkflowName: "workflow-dev",
			Force:        false,
		}
		res = post("/applications/"+appName+"/deploy", req)
		var response apisv1.ApplicationDeployResponse
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Status, model.RevisionStatusRunning)).Should(BeEmpty())

		var oam v1beta1.Application
		err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: appName, Namespace: envName}, &oam)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(oam.Spec.Components), 1)).Should(BeEmpty())
		Expect(cmp.Diff(len(oam.Spec.Policies), 1)).Should(BeEmpty())
	})

	It("Test recycling application", func() {
		var envName = "dev"
		res := post("/applications/"+appName+"/envs/"+envName+"/recycle", nil)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})

	It("Test create component", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateComponentRequest{
			Name:          "test2",
			Description:   "this is a test2 component",
			Labels:        map[string]string{},
			ComponentType: "worker",
			Properties:    `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
			DependsOn:     []string{"data-worker"},
		}
		res := post("/applications/"+appName+"/components", req)
		defer res.Body.Close()
		var response apisv1.ComponentBase
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.ComponentType, "worker")).Should(BeEmpty())
	})

	It("Test detail component", func() {
		defer GinkgoRecover()
		res := get("/applications/" + appName + "/components/test2")
		var response apisv1.DetailComponentResponse
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(len(response.DependsOn), 1)).Should(BeEmpty())
	})

	It("Test add trait", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationTraitRequest{
			Type:       "ingress",
			Properties: `{"domain": "www.test.com"}`,
		}
		res := post("/applications/"+appName+"/components/test2/traits", req)
		var response apisv1.ApplicationTrait
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Properties.JSON(), `{"domain":"www.test.com"}`)).Should(BeEmpty())
	})

	It("Test update trait", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationTraitRequest{
			Type:       "ingress",
			Properties: `{"domain": "www.test1.com"}`,
		}
		res := put("/applications/"+appName+"/components/test2/traits/ingress", req)
		var response apisv1.ApplicationTrait
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Properties.JSON(), `{"domain":"www.test1.com"}`)).Should(BeEmpty())
	})

	It("Test delete trait", func() {
		defer GinkgoRecover()
		res := delete("/applications/" + appName + "/components/test2/traits/ingress")
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})

	It("Test delete component", func() {
		defer GinkgoRecover()
		res := delete("/applications/" + appName + "/components/test2")
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})

	It("Test create application policy", func() {
		defer GinkgoRecover()
		var req = apisv1.CreatePolicyRequest{
			Name:        "test2",
			Description: "this is a test2 component",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		res := post("/applications/"+appName+"/policies", req)
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 400)).Should(BeEmpty())
		var req2 = apisv1.CreatePolicyRequest{
			Name:        "test2",
			Description: "this is a test2 policy",
			Type:        "wqsdasd",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		res = post("/applications/"+appName+"/policies", req2)
		var response apisv1.PolicyBase
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Type, "wqsdasd")).Should(BeEmpty())
	})

	It("Test detail application policy", func() {
		defer GinkgoRecover()
		res := get("/applications/" + appName + "/policies/test2")
		var response apisv1.DetailPolicyResponse
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Description, "this is a test2 policy")).Should(BeEmpty())
	})

	It("Test update application policy", func() {
		var req = apisv1.UpdatePolicyRequest{
			Description: "this is a test2 policy update",
			Type:        "wqsdasd",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		res := put("/applications/"+appName+"/policies/test2", req)
		var response apisv1.PolicyBase
		Expect(decodeResponseBody(res, &response)).Should(Succeed())
		Expect(cmp.Diff(response.Description, "this is a test2 policy update")).Should(BeEmpty())
	})

	It("Test delete application policy", func() {
		defer GinkgoRecover()
		res := delete("/applications/" + appName + "/policies/test2")
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})

	It("Test delete app", func() {
		defer GinkgoRecover()
		res := delete("/applications/" + appName)
		Expect(decodeResponseBody(res, nil)).Should(Succeed())
	})
})
