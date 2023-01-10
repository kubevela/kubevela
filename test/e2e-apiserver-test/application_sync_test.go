/*
Copyright 2022 The KubeVela Authors.

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
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/apiserver/domain/repository"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test the application synchronizing", func() {
	var appName = "test-synchronizing"
	It("Test create an application", func() {
		var app v1beta1.Application
		Expect(common.ReadYamlToObject("./testdata/example-app.yaml", &app)).Should(BeNil())
		app.Spec.Components[0].Name = appName
		app.Name = appName
		req := apisv1.ApplicationRequest{
			Components: app.Spec.Components,
			Policies:   app.Spec.Policies,
			Workflow:   app.Spec.Workflow,
		}
		res := post(fmt.Sprintf("/v1/namespaces/%s/applications/%s", "default", appName), req)
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
	})

	It("Test get the synchronizing application", func() {
		for retry := 0; retry < 5; retry++ {
			// Sleep 5 seconds to wait for the sync completed
			time.Sleep(time.Second * 5)
			res := get(fmt.Sprintf("/applications/%s", appName))
			Expect(res).ShouldNot(BeNil())
			if res.StatusCode == 404 {
				continue
			}
			var detail apisv1.DetailApplicationResponse
			Expect(decodeResponseBody(res, &detail)).Should(Succeed())
			Expect(cmp.Diff(len(detail.Policies), 3)).Should(BeEmpty())
			Expect(cmp.Diff(len(detail.EnvBindings), 1)).Should(BeEmpty())
			Expect(cmp.Diff(detail.ResourceInfo.ComponentNum, int64(2))).Should(BeEmpty())
			break
		}
	})

	It("Test get the synchronizing application revision", func() {
		res := get(fmt.Sprintf("/applications/%s/revisions", appName))
		Expect(res).ShouldNot(BeNil())
		var list apisv1.ListRevisionsResponse
		Expect(decodeResponseBody(res, &list)).Should(Succeed())
		Expect(cmp.Diff(len(list.Revisions), 1)).Should(BeEmpty())
	})

	It("Test get the synchronizing workflow record", func() {
		res := get(fmt.Sprintf("/applications/%s/records", appName))
		Expect(res).ShouldNot(BeNil())
		var list apisv1.ListWorkflowRecordsResponse
		Expect(decodeResponseBody(res, &list)).Should(Succeed())
		Expect(cmp.Diff(len(list.Records), 1)).Should(BeEmpty())
		Expect(cmp.Diff(len(list.Records[0].Steps), 3)).Should(BeEmpty())
	})

	It("Test change the publish version", func() {
		var app v1beta1.Application
		err := k8sClient.Get(context.TODO(), types.NamespacedName{
			Namespace: "default",
			Name:      appName,
		}, &app)
		Expect(err).Should(BeNil())
		oam.SetPublishVersion(&app, "test-v2")
		err = k8sClient.Update(context.TODO(), &app)
		Expect(err).Should(BeNil())

		Eventually(func() error {
			res := get(fmt.Sprintf("/applications/%s/revisions", appName))
			Expect(res).ShouldNot(BeNil())
			var list apisv1.ListRevisionsResponse
			Expect(decodeResponseBody(res, &list)).Should(Succeed())
			if len(list.Revisions) != 2 {
				return fmt.Errorf("the new revision is not synced")
			}
			recordRes := get(fmt.Sprintf("/applications/%s/workflows/%s/records", appName, repository.ConvertWorkflowName(list.Revisions[0].EnvName)))
			var lrr apisv1.ListWorkflowRecordsResponse
			Expect(decodeResponseBody(res, &recordRes)).Should(Succeed())
			Expect(lrr.Total).Should(Equal(int64(2)))
			Expect(lrr.Records[1].Name).Should(Equal("test-v2"))

			if list.Revisions[0].Status != "complete" {
				return fmt.Errorf("the new revision status is %s, record status is %s, not complete", list.Revisions[0].Status, lrr.Records[1].Status)
			}
			return nil
		}).WithTimeout(time.Minute * 1).WithPolling(3 * time.Second).Should(BeNil())
	})

	It("Test delete the application", func() {
		res := delete(fmt.Sprintf("/v1/namespaces/%s/applications/%s", "default", appName))
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

	It("Test get the application", func() {
		Eventually(func() error {
			res := get(fmt.Sprintf("/applications/%s", appName))
			if res == nil || res.StatusCode != 404 {
				return fmt.Errorf("failed to check the app status")
			}
			return nil
		}).WithTimeout(time.Minute * 1).WithPolling(2 * time.Second).Should(BeNil())
	})
})
