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
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test application rest api", func() {
	It("Test create app", func() {
		defer GinkgoRecover()
		var req = apisv1.CreateApplicationRequest{
			Name:        "test-app-sadasd",
			Namespace:   "test-app-namesapce",
			Description: "this is a test app",
			Icon:        "",
			Labels:      map[string]string{"test": "true"},
			ClusterList: []string{},
		}
		bodyByte, err := json.Marshal(req)
		Expect(err).ShouldNot(HaveOccurred())
		res, err := http.Post("http://127.0.0.1:8000/api/v1/applications", "application/json", bytes.NewBuffer(bodyByte))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(res).ShouldNot(BeNil())
		Expect(cmp.Diff(res.StatusCode, 200)).Should(BeEmpty())
		Expect(res.Body).ShouldNot(BeNil())
		defer res.Body.Close()
		var appBase apisv1.ApplicationBase
		err = json.NewDecoder(res.Body).Decode(&appBase)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cmp.Diff(appBase.Name, req.Name)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Description, req.Description)).Should(BeEmpty())
		Expect(cmp.Diff(appBase.Labels["test"], req.Labels["test"])).Should(BeEmpty())
	})
})
