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
)

var _ = Describe("Test system info  rest api", func() {

	It("Test get SystemInfo", func() {
		res := get("/system_info/")
		var info apisv1.SystemInfoResponse
		Expect(decodeResponseBody(res, &info)).Should(Succeed())
		Expect(len(info.PlatformID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(true))
		systemID := info.PlatformID

		// check several times the systemID should not change
		for i := 0; i < 5; i++ {
			res := get("/system_info/")
			var checkInfo apisv1.SystemInfoResponse
			Expect(decodeResponseBody(res, &checkInfo)).Should(Succeed())
			Expect(checkInfo.PlatformID).Should(BeEquivalentTo(systemID))
		}
	})

	It("Test disable/enable systemInfoCollection", func() {
		res := get("/system_info/")
		var info apisv1.SystemInfoResponse
		Expect(decodeResponseBody(res, &info)).Should(Succeed())
		Expect(len(info.PlatformID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(true))
		installID := info.PlatformID

		res = put("/system_info/", apisv1.SystemInfoRequest{EnableCollection: false})
		info = apisv1.SystemInfoResponse{}
		Expect(decodeResponseBody(res, &info)).Should(Succeed())
		Expect(len(info.PlatformID)).ShouldNot(BeEquivalentTo(0))
		Expect(info.EnableCollection).Should(BeEquivalentTo(false))

		res = get("/system_info/")
		var checkInfo apisv1.SystemInfoResponse
		Expect(decodeResponseBody(res, &checkInfo)).Should(Succeed())
		Expect(checkInfo.PlatformID).Should(BeEquivalentTo(installID))
		Expect(checkInfo.EnableCollection).Should(BeEquivalentTo(false))

		res = put("/system_info/", apisv1.SystemInfoRequest{EnableCollection: true})
		var enableInfo apisv1.SystemInfoResponse
		Expect(decodeResponseBody(res, &enableInfo)).Should(Succeed())
		Expect(len(enableInfo.PlatformID)).ShouldNot(BeEquivalentTo(0))
		Expect(enableInfo.EnableCollection).Should(BeEquivalentTo(true))
		Expect(enableInfo.PlatformID).Should(BeEquivalentTo(installID))

		res = get("/system_info/")
		var checkAgainInfo apisv1.SystemInfoResponse
		Expect(decodeResponseBody(res, &checkAgainInfo)).Should(Succeed())
		Expect(checkAgainInfo.PlatformID).Should(BeEquivalentTo(installID))
		Expect(checkAgainInfo.EnableCollection).Should(BeEquivalentTo(true))
	})
})
