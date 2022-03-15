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

package utils

import (
	"net/http"

	"github.com/emicklei/go-restful/v3"
	"github.com/google/go-cmp/cmp"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Test params utils", func() {
	It("Test ExtractPagingParams function", func() {
		req, err := http.NewRequest("GET", "/xx?page=2&pageSize=5", nil)
		Expect(err).Should(BeNil())
		page, pageSize, err := ExtractPagingParams(restful.NewRequest(req), 1, 15)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(page, 2)).Should(BeEmpty())
		Expect(cmp.Diff(pageSize, 5)).Should(BeEmpty())

		page, pageSize, err = ExtractPagingParams(restful.NewRequest(req), 1, 3)
		Expect(err).Should(BeNil())
		Expect(cmp.Diff(page, 2)).Should(BeEmpty())
		Expect(cmp.Diff(pageSize, 3)).Should(BeEmpty())
	})
})
