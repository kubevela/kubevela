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
	"fmt"
	"io"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/addon"
	apisv1 "github.com/oam-dev/kubevela/pkg/apiserver/interfaces/api/dto/v1"
)

var _ = Describe("Test addon rest api", func() {

	Describe("addon registry apiServer test", func() {
		It("list addon registry", func() {
			resp := get("/addon_registries")
			defer resp.Body.Close()
			var addonRegistry apisv1.ListAddonRegistryResponse
			Expect(decodeResponseBody(resp, &addonRegistry)).Should(Succeed())
			Expect(len(addonRegistry.Registries)).Should(BeEquivalentTo(2))
		})

		It("add addon registry", func() {
			req := apisv1.CreateAddonRegistryRequest{
				Name: "test-registry",
				Git: &addon.GitAddonSource{
					URL: "github.com/test-path",
				},
			}
			res := post("/addon_registries", req)
			defer res.Body.Close()
			var registry apisv1.AddonRegistry
			Expect(decodeResponseBody(res, &registry)).Should(Succeed())
			Expect(registry.Git).ShouldNot(BeNil())
			Expect(registry.Git.URL).Should(BeEquivalentTo("github.com/test-path"))

			resp := get("/addon_registries")
			var addonRegistry apisv1.ListAddonRegistryResponse
			Expect(decodeResponseBody(resp, &addonRegistry)).Should(Succeed())
			Expect(len(addonRegistry.Registries)).Should(BeEquivalentTo(3))
		})

		It("update an addon registry", func() {
			req := apisv1.UpdateAddonRegistryRequest{
				Git: &addon.GitAddonSource{
					URL: "github.com/another-path",
				},
			}
			res := put("/addon_registries"+"/test-registry", req)
			defer res.Body.Close()
			var registry apisv1.AddonRegistry
			Expect(decodeResponseBody(res, &registry)).Should(Succeed())
			Expect(registry.Git).ShouldNot(BeNil())
			Expect(registry.Git.URL).Should(BeEquivalentTo("github.com/another-path"))

			resp := get("/addon_registries")
			var addonRegistry apisv1.ListAddonRegistryResponse
			Expect(decodeResponseBody(resp, &addonRegistry)).Should(Succeed())
			Expect(len(addonRegistry.Registries)).Should(BeEquivalentTo(3))
			Expect(addonRegistry.Registries[2].Git.URL).Should(BeEquivalentTo("github.com/another-path"))
		})

		It("delete an addon registry", func() {
			res := delete("/addon_registries" + "/test-registry")
			defer res.Body.Close()
			var registry apisv1.AddonRegistry
			Expect(decodeResponseBody(res, &registry)).Should(Succeed())
		})
	})

	Describe("addon apiServer test", func() {
		It("list addons", func() {
			res := get("/addons")
			defer res.Body.Close()
			var addons apisv1.ListAddonResponse
			Expect(decodeResponseBody(res, &addons)).Should(Succeed())
			Expect(len(addons.Addons)).ShouldNot(BeEquivalentTo(0))
		})

		It("get addon detail", func() {
			res := get("/addons/mock-addon")
			defer res.Body.Close()
			var addon apisv1.DetailAddonResponse
			Expect(decodeResponseBody(res, &addon)).Should(Succeed())
			Expect(addon.Name).Should(BeEquivalentTo("mock-addon"))
			Expect(addon.Detail).Should(BeEquivalentTo("Test addon readme.md file"))
			Expect(len(addon.Definitions)).Should(BeEquivalentTo(1))
			Expect(addon.Definitions[0].Name).Should(BeEquivalentTo("kustomize-json-patch-mock-adddon"))
		})

		It("enable addon ", func() {
			req := apisv1.EnableAddonRequest{
				Args: map[string]interface{}{
					"testkey": "testvalue",
				},
			}
			res := post("/addons/mock-addon/enable", req)
			defer res.Body.Close()
			var addon apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addon)).Should(Succeed())
			Expect(addon.Name).Should(BeEquivalentTo("mock-addon"))
			Expect(len(addon.Args)).Should(BeEquivalentTo(1))
			Expect(addon.Args["testkey"]).Should(BeEquivalentTo("testvalue"))
		})

		It("addon status", func() {
			res := get("/addons/mock-addon/status")
			defer res.Body.Close()
			var addonStatus apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addonStatus)).Should(Succeed())
			Expect(addonStatus.Name).Should(BeEquivalentTo("mock-addon"))
			Expect(len(addonStatus.Args)).Should(BeEquivalentTo(1))
			Expect(addonStatus.Args["testkey"]).Should(BeEquivalentTo("testvalue"))
		})

		It("not enabled addon status", func() {
			res := get("/addons/example/status")
			defer res.Body.Close()
			var addonStatus apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addonStatus)).Should(Succeed())
			Expect(addonStatus.Name).Should(BeEquivalentTo("example"))
			Expect(addonStatus.Phase).Should(BeEquivalentTo("disabled"))
		})

		It("update addon ", func() {
			req := apisv1.EnableAddonRequest{
				Args: map[string]interface{}{
					"testkey": "new-testvalue",
				},
			}
			Eventually(func(g Gomega) {
				res := put("/addons/mock-addon/update", req)
				defer res.Body.Close()
				var addonStatus apisv1.AddonStatusResponse
				g.Expect(decodeResponseBody(res, &addonStatus)).Should(Succeed())
				g.Expect(addonStatus.Name).Should(BeEquivalentTo("mock-addon"))
				g.Expect(len(addonStatus.Args)).Should(BeEquivalentTo(1))
				g.Expect(addonStatus.Args["testkey"]).Should(BeEquivalentTo("new-testvalue"))

				status := get("/addons/mock-addon/status")
				var newaddonStatus apisv1.AddonStatusResponse
				g.Expect(decodeResponseBody(status, &newaddonStatus)).Should(Succeed())
				g.Expect(newaddonStatus.Name).Should(BeEquivalentTo("mock-addon"))
				g.Expect(len(newaddonStatus.Args)).Should(BeEquivalentTo(1))
				g.Expect(newaddonStatus.Args["testkey"]).Should(BeEquivalentTo("new-testvalue"))
			}, 15*time.Second).Should(Succeed())

		})

		It("list enabled addon", func() {
			Eventually(func() error {
				res := get("/enabled_addon/")
				defer res.Body.Close()
				var addonList apisv1.ListEnabledAddonResponse
				err := decodeResponseBody(res, &addonList)
				if err != nil {
					return err
				}
				if len(addonList.EnabledAddons) == 0 {
					return fmt.Errorf("error number")
				}
				return nil
			}, 30*time.Second, 300*time.Millisecond).Should(BeNil())
		})

		It("disable addon ", func() {
			res := post("/addons/mock-addon/disable", nil)
			defer res.Body.Close()
			var addonStatus apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addonStatus)).Should(Succeed())
			Expect(addonStatus.Name).Should(BeEquivalentTo("mock-addon"))
		})

		It("enable addon with not match system version requirement", func() {
			req := apisv1.EnableAddonRequest{
				Args: map[string]interface{}{
					"testkey": "testvalue",
				},
			}
			res := post("/addons/not-match-addon/enable", req)
			defer res.Body.Close()
			type errResp struct {
				BusinessCode int    `json:"BusinessCode"`
				Message      string `json:"Message"`
			}
			var errResponse errResp
			body, err := io.ReadAll(res.Body)
			Expect(err).Should(BeNil())
			err = json.Unmarshal(body, &errResponse)
			Expect(err).Should(BeNil())
			Expect(errResponse.BusinessCode).Should(BeEquivalentTo(50018))
			Expect(strings.Contains(errResponse.Message, "fail to install"))
		})
	})

	Describe("Test addon dependency installed version", func() {
		It("Test Operation of enabling foo addon will enable bar addon automatically", func() {
			req := apisv1.EnableAddonRequest{}
			res := post("/addons/foo/enable", req)
			defer res.Body.Close()
			var addon apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addon)).Should(Succeed())
			Expect(addon.Name).Should(BeEquivalentTo("foo"))

			Eventually(func(g Gomega) {
				status := get("/addons/bar/status")
				var newaddonStatus apisv1.AddonStatusResponse
				g.Expect(decodeResponseBody(status, &newaddonStatus)).Should(Succeed())
				g.Expect(newaddonStatus.Name).Should(BeEquivalentTo("bar"))
				g.Expect(newaddonStatus.InstalledVersion).Should(BeEquivalentTo("v1.0.0"))
			}, 30*time.Second, 300*time.Millisecond).Should(Succeed())
		})
	})

	Describe("Test addon dependency addon in other registry", func() {
		It("Test Operation of enable addon from other registry", func() {
			req := apisv1.EnableAddonRequest{}
			res := post("/addons/mock-dep-addon/enable", req)
			defer res.Body.Close()
			var addon apisv1.AddonStatusResponse
			Expect(decodeResponseBody(res, &addon)).Should(Succeed())
			Expect(addon.Name).Should(BeEquivalentTo("mock-dep-addon"))

			Eventually(func(g Gomega) {
				status := get("/addons/mock-dep-addon/status")
				var newaddonStatus apisv1.AddonStatusResponse
				g.Expect(decodeResponseBody(status, &newaddonStatus)).Should(Succeed())
				g.Expect(newaddonStatus.Name).Should(BeEquivalentTo("mock-dep-addon"))
				g.Expect(newaddonStatus.InstalledVersion).Should(BeEquivalentTo("v1.0.0"))
				g.Expect(newaddonStatus.Phase).Should(BeEquivalentTo(apisv1.AddonPhaseEnabled))
			}, 30*time.Second, 300*time.Millisecond).Should(Succeed())
		})
	})

	Describe("Test enable an addon with specified registry", func() {
		It("Test with a not exist registry", func() {
			req := apisv1.EnableAddonRequest{
				RegistryName: "not-exist",
			}
			res := post("/addons/test-addon/enable", req)
			defer res.Body.Close()
			Expect(res.StatusCode).Should(BeEquivalentTo(400))
		})
	})
})
