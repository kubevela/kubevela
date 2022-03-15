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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
)

var _ = Describe("Test http utils", func() {
	It("Test get ClientIP function", func() {
		req, err := http.NewRequest("GET", "/xx?page=2&pageSize=5", nil)
		Expect(err).Should(BeNil())
		req.Header.Set("X-Real-Ip", "198.23.1.1")
		clientIP := ClientIP(req)
		Expect(cmp.Diff(clientIP, "198.23.1.1")).Should(BeEmpty())

		req.Header.Set("X-Forwarded-For", "198.23.1.2")
		clientIP = ClientIP(req)
		Expect(cmp.Diff(clientIP, "198.23.1.2")).Should(BeEmpty())
	})
})
