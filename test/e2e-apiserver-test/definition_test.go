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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
	"github.com/oam-dev/kubevela/pkg/apiserver/rest/utils"
)

var _ = Describe("Test definitions rest api", func() {

	It("Test list definitions", func() {
		defer GinkgoRecover()
		res := get("/definitions?type=component")
		var definitions apisv1.ListDefinitionResponse
		Expect(decodeResponseBody(res, &definitions)).Should(Succeed())
	})

	It("Test detail the definition", func() {
		defer GinkgoRecover()
		res := get("/definitions/webservice")
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
		Expect(res.Status).Should(Equal(400))

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
		Expect(res2.Status).Should(Equal(400))
	})

})
