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
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/addon"
	apis "github.com/oam-dev/kubevela/pkg/apiserver/rest/apis/v1"
)

var _ = Describe("Test addon rest api", func() {
	registryName := "test-addon-registry"
	createReq := apis.CreateAddonRegistryRequest{
		Name: registryName,
		Oss: &addon.OSSAddonSource{
			Endpoint: "https://oss-cn-hangzhou.aliyuncs.com",
			Bucket:   "fake-kubevela-addons",
		},
	}
	It("should add and delete a registry, list addons from default registry", func() {
		defer GinkgoRecover()

		By("add registry")
		createRes := post("/addon_registries", createReq)
		Expect(createRes).ShouldNot(BeNil())
		Expect(createRes.Body).ShouldNot(BeNil())
		Expect(createRes.StatusCode).Should(Equal(200))

		defer createRes.Body.Close()

		var rmeta apis.AddonRegistry
		err := json.NewDecoder(createRes.Body).Decode(&rmeta)
		Expect(err).Should(BeNil())
		Expect(rmeta.Name).Should(Equal(createReq.Name))
		Expect(rmeta.Git).Should(Equal(createReq.Git))
		Expect(rmeta.OSS).Should(Equal(createReq.Oss))

		deleteRes := delete("/addon_registries/" + createReq.Name)
		Expect(deleteRes).ShouldNot(BeNil())
		Expect(deleteRes.StatusCode).Should(Equal(200))
	})

	It("list addons", func() {
		DefaultRegistry := "KubeVela"
		listRes := get("/addons/")
		defer listRes.Body.Close()

		var lres apis.ListAddonResponse
		err := json.NewDecoder(listRes.Body).Decode(&lres)
		Expect(err).Should(BeNil())
		Expect(lres.Addons).ShouldNot(BeZero())
		Expect(lres.Addons[0].RegistryName).To(Equal(DefaultRegistry))

		By("get addon detail")
		detailRes := get("/addons/terraform-alibaba")
		defer detailRes.Body.Close()

		var dres *apis.DetailAddonResponse
		err = json.NewDecoder(detailRes.Body).Decode(&dres)
		Expect(err).Should(BeNil())
		Expect(dres.Meta).ShouldNot(BeNil())
		Expect(dres.UISchema).ShouldNot(BeNil())
		Expect(dres.APISchema).ShouldNot(BeNil())
		Expect(dres.RegistryName).Should(Equal(DefaultRegistry))
	})

	PIt("should enable and disable an addon", func() {
		defer GinkgoRecover()
		req := apis.EnableAddonRequest{
			Args: map[string]interface{}{
				"example": "test-args",
			},
		}
		testAddon := "example"
		res := post("/addons/"+testAddon+"/enable", req)
		defer res.Body.Close()
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		defer res.Body.Close()

		var statusRes apis.AddonStatusResponse
		err := json.NewDecoder(res.Body).Decode(&statusRes)

		Expect(err).Should(BeNil())
		Expect(statusRes.Phase).Should(Equal(apis.AddonPhaseEnabling))

		// Wait for addon enabled

		period := 30 * time.Second
		timeout := 2 * time.Minute
		Eventually(func() error {
			res = get("/addons/" + testAddon + "/status")
			defer res.Body.Close()
			err = json.NewDecoder(res.Body).Decode(&statusRes)
			Expect(err).Should(BeNil())
			if statusRes.Phase == apis.AddonPhaseEnabled {
				return nil
			}
			return errors.New("not ready")
		}, timeout, period).Should(BeNil())

		res = post("/addons/"+testAddon+"/disable", req)
		defer res.Body.Close()
		Expect(res).ShouldNot(BeNil())
		Expect(res.StatusCode).Should(Equal(200))
		Expect(res.Body).ShouldNot(BeNil())

		err = json.NewDecoder(res.Body).Decode(&statusRes)
		Expect(err).Should(BeNil())
	})

	It("should delete test registry", func() {
		defer GinkgoRecover()
	})
})
