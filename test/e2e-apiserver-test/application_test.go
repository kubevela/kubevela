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

var _ = Describe("Test application rest api", func() {
	It("Test create app", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationRequest{
			Name:        "test-app-sadasd",
			Namespace:   "test-app-namesapce",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			ClusterList: []string{},
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
		req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/v1/applications/test-app-sadasd", nil)
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
			Name:        "test-app-sadasd",
			Namespace:   "test-app-namesapce",
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
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/components")
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

	It("Test list policies", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var policies apisv1.ListApplicationPolicy
		err = json.NewDecoder(res.Body).Decode(&policies)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(len(policies.Policies), 1)).Should(BeEmpty())
	})

	It("Test get workflow", func() {
		// defer GinkgoRecover()
		// res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies")
		// Expect(err).ShouldNot(HaveOccurred())
		// Expect(res).ShouldNot(BeNil())
		// Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		// Expect(res.Body).ShouldNot(BeNil())
		// defer res.Body.Close()
		// var policies apisv1.ListApplicationPolicy
		// err = json.NewDecoder(res.Body).Decode(&policies)
		// Expect(err).ShouldNot(HaveOccurred())
		// Expect(cmp.Diff(len(policies.Policies), 1)).Should(BeEmpty())
	})

	It("Test detail application", func() {
		defer GinkgoRecover()
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd")
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var detail apisv1.DetailApplicationResponse
		err = json.NewDecoder(res.Body).Decode(&detail)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(len(detail.Policies), 1)).Should(BeEmpty())
	})

	It("Test deploy application", func() {
		defer GinkgoRecover()
		var req = apisv1.ApplicationDeployRequest{
			Commit:     "test apply",
			SourceType: "web",
			Force:      false,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/deploy", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var response apisv1.ApplicationDeployResponse
		err = json.NewDecoder(res.Body).Decode(&response)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(response.Status, model.DeployEventRunning)).Should(BeEmpty())

		var oam v1beta1.Application
		err = k8sClient.Get(context.TODO(), types.NamespacedName{Name: "test-app-sadasd", Namespace: "test-app-namesapce"}, &oam)
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
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/components", "application/json", bytes.NewBuffer(bodyByte))
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
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/components/test2")
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

	It("Test create application policy", func() {
		defer GinkgoRecover()
		var req = apisv1.CreatePolicyRequest{
			Name:        "test2",
			Description: "this is a test2 component",
			Properties:  `{"image": "busybox","cmd":["sleep", "1000"],"lives": "3","enemies": "alien"}`,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies", "application/json", bytes.NewBuffer(bodyByte))
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
		res, err = http.Post("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies", "application/json", bytes.NewBuffer(bodyByte2))
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
		res, err := http.Get("http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies/test2")
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
		req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies/test2", bytes.NewBuffer(bodyByte2))
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
		req, err := http.NewRequest(http.MethodDelete, "http://127.0.0.1:8000/api/v1/applications/test-app-sadasd/policies/test2", nil)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

})
