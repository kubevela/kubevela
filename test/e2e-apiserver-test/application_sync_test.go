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
	"fmt"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
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

	It("Test delete the application", func() {
		res := delete(fmt.Sprintf("/v1/namespaces/%s/applications/%s", "default", appName))
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
	})

	It("Test get the application", func() {
		// Sleep 5 seconds to wait for the sync completed
		time.Sleep(time.Second * 5)
		res := get(fmt.Sprintf("/applications/%s", appName))
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(404))
	})
})
