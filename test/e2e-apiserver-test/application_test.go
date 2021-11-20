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
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

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
			Namespace:   appProject,
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			EnvBinding:  []*apisv1.EnvBinding{{Name: "dev-env", TargetNames: []string{"test-target"}}},
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var appBase apisv1.ApplicationBase
		err = json.NewDecoder(res.Body).Decode(&appBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(appBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Namespace, req.Namespace)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Labels["test"], req.Labels["test"])).Should(BeEmpty())
	})

	It("Test delete app", func() {
		defer GinkgoRecover()
		req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/v1/applications/"+appName, nil)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

	It("Test create app with oamspec", func() {
		defer GinkgoRecover()
		bs, err := ioutil.ReadFile("./testdata/example-app.yaml")
		Expect(err).Should(Succeed())
		var req = apisv1.CreateApplicationRequest{
			Name:        appName,
			Namespace:   appProject,
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			YamlConfig:  string(bs),
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var appBase apisv1.ApplicationBase
		err = json.NewDecoder(res.Body).Decode(&appBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(appBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Namespace, req.Namespace)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Labels["test"], req.Labels["test"])).Should(BeEmpty())
	})

	It("Test list components", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/" + appName + "/components")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var components apisv1.ComponentListResponse
		err = json.NewDecoder(res.Body).Decode(&components)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(len(components.Components), 2)).Should(BeEmpty())
	})

	It("Test detail application", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/" + appName)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var detail apisv1.DetailApplicationResponse
		err = json.NewDecoder(res.Body).Decode(&detail)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(len(detail.Policies), 0)).Should(BeEmpty())
	})

	It("Test deploy application", func() {
		defer GinkgoRecover()
		var targetName = "dev-default"
		var envName = "dev"
		var namespace = "default"
		// create target
		var createTarget = apisv1.CreateDeliveryTargetRequest{
			Name:      targetName,
			Namespace: appProject,
			Cluster: &apisv1.ClusterTarget{
				ClusterName: "local",
				Namespace:   namespace,
			},
		}
		bodyByte, err := json.Marshal(createTarget)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/deliveryTargets", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())

		// create env
		var createEnvReq = apisv1.CreateApplicationEnvRequest{
			EnvBinding: apisv1.EnvBinding{
				Name:        envName,
				TargetNames: []string{targetName},
			},
		}
		bodyByte, err = json.Marshal(createEnvReq)
		Expect(err).ShouldNot(HaveOccurred())
		res, err = http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/envs", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())

		// deploy app
		var req = apisv1.ApplicationDeployRequest{
			Note:         "test apply",
			TriggerType:  "web",
			WorkflowName: "dev",
			Force:        false,
		}
		bodyByte, err = json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err = http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/deploy", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.ApplicationDeployResponse
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Status, model.RevisionStatusRunning)).Should(BeEmpty())

		var oam v1beta1.Application
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: appName + "-" + envName, Namespace: appProject}, &oam)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(len(oam.Spec.Components), 2)).Should(BeEmpty())
		Expect(cmp.Diff(len(oam.Spec.Policies), 1)).Should(BeEmpty())
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
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/components", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.ComponentBase
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.ComponentType, "worker")).Should(BeEmpty())
	})

	It("Test detail component", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/" + appName + "/components/test2")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.DetailComponentResponse
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(len(response.DependsOn), 1)).Should(BeEmpty())
	})

	It("Test add trait", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationTraitRequest{
			Type:       "ingress",
			Properties: `{"domain": "www.test.com"}`,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/components/test2/traits", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.ApplicationTrait
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Properties.JSON(), `{"domain":"www.test.com"}`)).Should(BeEmpty())
	})

	It("Test update trait", func() {
		defer GinkgoRecover()
		var req2 = apisv1.CreateApplicationTraitRequest{
			Type:       "ingress",
			Properties: `{"domain": "www.test1.com"}`,
		}
		bodyByte, err := json.Marshal(req2)
		Expect(err).ShouldNot(HaveOccurred())
		req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:8000/api/v1/applications/"+appName+"/components/test2/traits/ingress", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.ApplicationTrait
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Properties.JSON(), `{"domain":"www.test1.com"}`)).Should(BeEmpty())
	})

	It("Test delete trait", func() {
		defer GinkgoRecover()
		req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/v1/applications/"+appName+"/components/test2/traits/ingress", nil)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

	It("Test create application policy", func() {
		defer GinkgoRecover()
		var req = apisv1.CreatePolicyRequest{
			Name:        "test2",
			Description: "this is a test2 component",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/policies", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 400)).Should(BeEmpty())
		var req2 = apisv1.CreatePolicyRequest{
			Name:        "test2",
			Description: "this is a test2 policy",
			Type:        "wqsdasd",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		bodyByte2, err := json.Marshal(req2)
		Expect(err).ShouldNot(HaveOccurred())
		res, err = http.Post("http://127.0.0.1:8000/api/v1/applications/"+appName+"/policies", "application/json", bytes.NewBuffer(bodyByte2))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())

		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.PolicyBase
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Type, "wqsdasd")).Should(BeEmpty())
	})

	It("Test detail application policy", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/" + appName + "/policies/test2")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.DetailPolicyResponse
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Description, "this is a test2 policy")).Should(BeEmpty())
	})

	It("Test update application policy", func() {
		var req2 = apisv1.UpdatePolicyRequest{
			Description: "this is a test2 policy update",
			Type:        "wqsdasd",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		bodyByte2, err := json.Marshal(req2)
		Expect(err).ShouldNot(HaveOccurred())
		req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:8000/api/v1/applications/"+appName+"/policies/test2", bytes.NewBuffer(bodyByte2))
		Expect(err).ShouldNot(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())

		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.PolicyBase
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Description, "this is a test2 policy update")).Should(BeEmpty())
	})

	It("Test delete application policy", func() {
		defer GinkgoRecover()
		req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/v1/applications/"+appName+"/policies/test2", nil)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

})
