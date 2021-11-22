/*
 Copyright 2021. The KubeVela Authors.

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
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test oam application rest api", func() {
	namespace := "test-oam-app"
	appName := "example-app"
	var app v1beta1.Application

	It("Test create and update oam app", func() {
		defer GinkgoRecover()
		By("test create app")

		Expect(common.ReadYamlToObject("./testdata/example-app.yaml", &app)).Should(BeNil())
		req := apiv1.ApplicationRequest{
			Components: app.Spec.Components,
			Policies:   app.Spec.Policies,
			Workflow:   app.Spec.Workflow,
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).Should(BeNil())
		res, err := http.Post(
			fmt.Sprintf("http://127.0.0.1:8000/v1/namespaces/%s/applications/%s", namespace, appName),
			"application/json",
			bytes.NewBuffer(bodyByte),
		)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()

		ctx := context.Background()
		oldApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, oldApp)).Should(BeNil())
		Expect(oldApp.Spec.Components).Should(Equal(req.Components))
		Expect(oldApp.Spec.Policies).Should(Equal(req.Policies))
		Expect(oldApp.Spec.Workflow).Should(Equal(req.Workflow))

		By("test update app")
		updateReq := apiv1.ApplicationRequest{
			Components: app.Spec.Components[1:],
		}
		bodyByte, err = json.Marshal(updateReq)
		Expect(err).Should(BeNil())
		Eventually(func(g Gomega) {
			res, err = http.Post(
				fmt.Sprintf("http://127.0.0.1:8000/v1/namespaces/%s/applications/%s", namespace, appName),
				"application/json",
				bytes.NewBuffer(bodyByte),
			)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(res).ShouldNot(BeNil())
			g.Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
			g.Expect(res.Body).ShouldNot(BeNil())
			defer res.Body.Close()
		}, time.Minute).Should(Succeed())
		newApp := new(v1beta1.Application)
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: appName, Namespace: namespace}, newApp)).Should(BeNil())
		Expect(newApp.Spec.Components).Should(Equal(updateReq.Components))
		Expect(newApp.Spec.Policies).Should(BeNil())
		Expect(newApp.Spec.Workflow).Should(BeNil())
	})

	It("Test get oam app", func() {
		defer GinkgoRecover()
		res, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/v1/namespaces/%s/applications/%s", namespace, appName),
		)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())

		defer res.Body.Close()
		var appResp apiv1.ApplicationResponse
		err = json.NewDecoder(res.Body).Decode(&appResp)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(len(appResp.Spec.Components)).Should(Equal(1))
	})

	It("Test delete oam app", func() {
		defer GinkgoRecover()
		req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://127.0.0.1:8000/v1/namespaces/%s/applications/%s", namespace, appName), nil)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.DefaultClient.Do(req)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})
})
