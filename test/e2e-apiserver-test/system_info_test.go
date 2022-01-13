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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

func put(path string, body interface{}) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).Should(BeNil())

	req, err := http.NewRequest(http.MethodPut, baseURL+path, bytes.NewBuffer(b))
	Expect(err).Should(BeNil())
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	Expect(err).Should(BeNil())
	Expect(res).ShouldNot(BeNil())
	Expect(res.StatusCode).Should(Equal(200))
	return res
}

var _ = Describe("Test system info  rest api", func() {
	BeforeEach(func() {
		req, err := http.NewRequest(http.MethodDelete, baseURL+"/api/v1/system_info/", nil)
		Expect(err).Should(BeNil())
		deleteRes, err := http.DefaultClient.Do(req)
		Expect(err).Should(BeNil())
		Expect(deleteRes).ShouldNot(BeNil())
		Expect(deleteRes.StatusCode).Should(Equal(200))
	})

	It("Test get SystemInfo", func() {
		response := get("/api/v1/system_info/")
		Expect(response).ShouldNot(BeNil())
		Expect(response.Body).ShouldNot(BeNil())
		Expect(response.StatusCode).Should(Equal(200))

		defer response.Body.Close()

		var info apisv1.SystemInfoResponse
		err := json.NewDecoder(response.Body).Decode(&info)
		Expect(err).Should(BeNil())
		Expect(len(info.InstallID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(true))
		systemID := info.InstallID

		// check several times the systemID should not change
		for i := 0; i < 5; i++ {
			check := get("/api/v1/system_info/")
			Expect(check).ShouldNot(BeNil())
			Expect(check.Body).ShouldNot(BeNil())
			Expect(check.StatusCode).Should(Equal(200))

			var checkInfo apisv1.SystemInfoResponse
			err := json.NewDecoder(check.Body).Decode(&checkInfo)
			Expect(err).Should(BeNil())
			Expect(checkInfo.InstallID).Should(BeEquivalentTo(systemID))
		}
	})

	It("Test disable/enable systemInfoCollection", func() {
		response := get("/api/v1/system_info/")
		Expect(response).ShouldNot(BeNil())
		Expect(response.Body).ShouldNot(BeNil())
		Expect(response.StatusCode).Should(Equal(200))

		defer response.Body.Close()

		var info apisv1.SystemInfoResponse
		err := json.NewDecoder(response.Body).Decode(&info)
		Expect(err).Should(BeNil())
		Expect(len(info.InstallID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(true))
		installID := info.InstallID

		response = put("/api/v1/system_info/", apisv1.SystemInfoRequest{EnableCollection: false})
		info = apisv1.SystemInfoResponse{}
		err = json.NewDecoder(response.Body).Decode(&info)
		Expect(err).Should(BeNil())
		Expect(len(info.InstallID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(false))

		getRes := get("/api/v1/system_info/")
		Expect(getRes).ShouldNot(BeNil())
		Expect(getRes.Body).ShouldNot(BeNil())
		Expect(getRes.StatusCode).Should(Equal(200))

		var checkInfo apisv1.SystemInfoResponse
		err = json.NewDecoder(getRes.Body).Decode(&checkInfo)
		Expect(err).Should(BeNil())
		Expect(checkInfo.InstallID).Should(BeEquivalentTo(installID))
		Expect(checkInfo.EnableCollection).Should(BeEquivalentTo(false))

		response = put("/api/v1/system_info/", apisv1.SystemInfoRequest{EnableCollection: true})
		Expect(response).ShouldNot(BeNil())
		Expect(response.StatusCode).Should(Equal(200))

		var enableInfo apisv1.SystemInfoResponse
		err = json.NewDecoder(response.Body).Decode(&enableInfo)
		Expect(err).Should(BeNil())
		Expect(len(enableInfo.InstallID)).ShouldNot(BeEquivalentTo(0))
		Expect(enableInfo.EnableCollection).Should(BeEquivalentTo(true))
		Expect(enableInfo.InstallID).Should(BeEquivalentTo(installID))

		getAgainRes := get("/api/v1/system_info/")
		Expect(getRes).ShouldNot(BeNil())
		Expect(getRes.Body).ShouldNot(BeNil())
		Expect(getRes.StatusCode).Should(Equal(200))

		var checkAgainInfo apisv1.SystemInfoResponse
		err = json.NewDecoder(getAgainRes.Body).Decode(&checkAgainInfo)
		Expect(err).Should(BeNil())
		Expect(checkAgainInfo.InstallID).Should(BeEquivalentTo(installID))
		Expect(checkAgainInfo.EnableCollection).Should(BeEquivalentTo(true))
	})
})
