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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	apiv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test velaQL rest api", func() {
	namespace := "test-velaql"
	appName := "example-app"
	var app v1beta1.Application

	It("Test query application status via view", func() {
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
		Expect(res.StatusCode).Should(Equal(200))

		Expect(err).Should(BeNil())
		queryRes, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{name=%s,namespace=%s}.%s", "read-object", appName, namespace, "output.value.spec"),
		)
		Expect(err).Should(BeNil())
		Expect(queryRes.StatusCode).Should(Equal(200))

		defer queryRes.Body.Close()
		var appSpec v1beta1.ApplicationSpec
		err = json.NewDecoder(queryRes.Body).Decode(&appSpec)
		Expect(err).ShouldNot(HaveOccurred())

		var existApp v1beta1.Application
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: appName, Namespace: namespace}, &existApp)).Should(BeNil())

		Expect(len(appSpec.Components)).Should(Equal(len(existApp.Spec.Components)))
		Expect(len(appSpec.Workflow.Steps)).Should(Equal(len(existApp.Spec.Workflow.Steps)))
	})

	It("Test query application status with wrong velaQL", func() {
		queryRes, err := http.Get(
			fmt.Sprintf("http://127.0.0.1:8000/api/v1/query?velaql=%s{err=,name=%s,namespace=%s}.%s", "read-object", appName, namespace, "output.value.spec"),
		)
		Expect(err).Should(BeNil())
		Expect(queryRes.StatusCode).Should(Equal(400))
	})
})
