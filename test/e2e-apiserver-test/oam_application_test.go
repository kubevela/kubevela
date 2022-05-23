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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
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
		res := post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appName), req)
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
		Eventually(func(g Gomega) {
			res = post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appName), updateReq)
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
		res := get(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appName))
		Expect(res).ShouldNot(BeNil())

		defer res.Body.Close()
		var appResp apiv1.ApplicationResponse
		err := json.NewDecoder(res.Body).Decode(&appResp)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(len(appResp.Spec.Components)).Should(Equal(1))
	})

	It("Test delete oam app", func() {
		defer GinkgoRecover()
		res := delete(fmt.Sprintf("/v1/namespaces/%s/applications/%s", namespace, appName))
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})
})
