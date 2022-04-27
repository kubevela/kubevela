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
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test definitions rest api", func() {

	It("Test list definitions", func() {
		defer GinkgoRecover()
		res := get("/definitions?type=component")
		var definitions apisv1.ListDefinitionResponse
		Expect(decodeResponseBody(res, &definitions)).Should(Succeed())
		Expect(len(definitions.Definitions) > 0).Should(BeTrue())

		updateDefinitionName := definitions.Definitions[0].Name
		upRes := put(fmt.Sprintf("/definitions/%s/status", updateDefinitionName), apisv1.UpdateDefinitionStatusRequest{
			DefinitionType: "component",
			HiddenInUI:     true,
		})
		Expect(upRes.StatusCode).Should(Equal(200))

		res2 := get("/definitions?type=component&queryAll=true")
		var allDefinitions apisv1.ListDefinitionResponse
		Expect(decodeResponseBody(res2, &allDefinitions)).Should(Succeed())
		expected := false
		for _, d := range allDefinitions.Definitions {
			if d.Name == updateDefinitionName && d.Status == "disable" {
				expected = true
			}
		}
		Expect(expected).Should(BeTrue())
	})

	It("Test detail the definition", func() {
		defer GinkgoRecover()
		componentSchema := new(corev1.ConfigMap)
		Expect(common.ReadYamlToObject("./testdata/component-schema-webservice.yaml", componentSchema)).Should(BeNil())
		Expect(k8sClient.Create(context.Background(), componentSchema)).Should(SatisfyAny(BeNil(), &util.AlreadyExistMatcher{}))
		res := get("/definitions/webservice?type=component")
		var detail apisv1.DetailDefinitionResponse
		Expect(decodeResponseBody(res, &detail)).Should(Succeed())
	})

	It("Test update ui schema", func() {
		defer GinkgoRecover()
		req := apisv1.UpdateUISchemaRequest{
			DefinitionType: "component",
			UISchema: utils.UISchema{
				{
					JSONKey: "image",
					UIType:  "ImageInput",
				},
			},
		}
		res := put("/definitions/webservice/uischema", req)
		var schema utils.UISchema
		Expect(decodeResponseBody(res, &schema)).Should(Succeed())
	})

	It("Test error update ui schema", func() {
		defer GinkgoRecover()
		req := apisv1.UpdateUISchemaRequest{
			DefinitionType: "component",
			UISchema: utils.UISchema{
				{
					JSONKey: "image",
					UIType:  "ImageInput",
					Conditions: []utils.Condition{
						{
							JSONKey: "",
						},
					},
				},
			},
		}
		res := put("/definitions/webservice/uischema", req)
		Expect(res.StatusCode).Should(Equal(400))

		req2 := apisv1.UpdateUISchemaRequest{
			DefinitionType: "component",
			UISchema: utils.UISchema{
				{
					JSONKey: "image",
					UIType:  "ImageInput",
					Conditions: []utils.Condition{
						{
							JSONKey: "secretName",
							Value:   "",
							Op:      "===",
						},
					},
				},
			},
		}
		res2 := put("/definitions/webservice/uischema", req2)
		Expect(res2.StatusCode).Should(Equal(400))
	})

})
