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

package schema

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test ui schema utils", func() {
	It("Test GetDefaultUIType function", func() {
		testCase := []map[string]interface{}{{
			"apiType":     "string",
			"haveOptions": true,
			"subType":     "",
			"haveSub":     true,
			"result":      "Select",
		}, {
			"apiType":     "string",
			"haveOptions": false,
			"subType":     "",
			"haveSub":     true,
			"result":      "Input",
		}, {
			"apiType":     "number",
			"haveOptions": false,
			"subType":     "",
			"haveSub":     true,
			"result":      "Number",
		}, {
			"apiType":     "integer",
			"haveOptions": false,
			"subType":     "",
			"haveSub":     true,
			"result":      "Number",
		}, {
			"apiType":     "boolean",
			"haveOptions": false,
			"subType":     "",
			"haveSub":     true,
			"result":      "Switch",
		},
			{
				"apiType":     "array",
				"haveOptions": false,
				"subType":     "string",
				"haveSub":     true,
				"result":      "Strings",
			},
			{
				"apiType":     "array",
				"haveOptions": false,
				"subType":     "",
				"haveSub":     true,
				"result":      "Structs",
			},
			{
				"apiType":     "object",
				"haveOptions": false,
				"subType":     "",
				"haveSub":     true,
				"result":      "Group",
			},
			{
				"apiType":     "object",
				"haveOptions": false,
				"subType":     "",
				"haveSub":     false,
				"result":      "KV",
			},
		}
		for _, tc := range testCase {
			uiType := GetDefaultUIType(tc["apiType"].(string), tc["haveOptions"].(bool), tc["subType"].(string), tc["haveSub"].(bool))
			Expect(uiType).Should(Equal(tc["result"]))
		}
	})
})
