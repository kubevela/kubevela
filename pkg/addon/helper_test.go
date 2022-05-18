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

package addon

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("test FindWholeAddonPackagesFromRegistry", func() {
	Describe("when no registry is added, no matter what you do, it will just return error", func() {
		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{}, []string{})
				Expect(err).To(HaveOccurred())
			})
			It("should return error", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, nil, nil)
				Expect(err).To(HaveOccurred())
			})
		})
		Context("when non-empty addonNames and registryNames is supplied", func() {
			It("should return error saying ErrRegistryNotExist", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"fluxcd"}, []string{"some-registry"})
				fmt.Println(err)
				Expect(errors.Is(err, ErrRegistryNotExist)).To(BeTrue())
			})
		})
	})

	Describe("one versioned registry is added", func() {
		BeforeEach(func() {
			// Prepare KubeVela registry
			reg := &Registry{
				Name: "KubeVela",
				Helm: &HelmSource{
					URL: "https://addons.kubevela.net",
				},
			}
			ds := NewRegistryDataStore(k8sClient)
			Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up KubeVela registry
			ds := NewRegistryDataStore(k8sClient)
			Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
		})

		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{}, []string{"KubeVela"})
				Expect(err).To(HaveOccurred())
			})
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, nil, []string{"KubeVela"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("one existing addon name provided", func() {
			It("should return one valid result, matching all registries", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"velaux"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("velaux"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
				Expect(res[0].APISchema).ToNot(BeNil())
			})
			It("should return one valid result, matching one registry", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"velaux"}, []string{"KubeVela"})
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("velaux"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
				Expect(res[0].APISchema).ToNot(BeNil())
			})
		})

		Context("one non-existent addon name provided", func() {
			It("should return error as ErrNotExist", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"non-existent-addon"}, nil)
				Expect(errors.Is(err, ErrNotExist)).To(BeTrue())
				Expect(res).To(BeNil())
			})
		})

		Context("two existing addon names provided", func() {
			It("should return two valid result", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"velaux", "traefik"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(2))
				Expect(res[0].Name).To(Equal("velaux"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
				Expect(res[0].APISchema).ToNot(BeNil())
				Expect(res[1].Name).To(Equal("traefik"))
				Expect(res[1].InstallPackage).ToNot(BeNil())
				Expect(res[1].APISchema).ToNot(BeNil())
			})
		})

		Context("one existing addon name and one non-existent addon name provided", func() {
			It("should return only one valid result", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"velaux", "non-existent-addon"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("velaux"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
				Expect(res[0].APISchema).ToNot(BeNil())
			})
		})
	})

	Describe("one non-versioned registry is added", func() {
		var server *httptest.Server
		BeforeEach(func() {
			// Prepare local non-versioned registry
			server = httptest.NewServer(ossHandler)
			cm := v1.ConfigMap{}
			cmYaml := strings.ReplaceAll(registryCmYaml, "TEST_SERVER_URL", server.URL)
			cmYaml = strings.ReplaceAll(cmYaml, "KubeVela", "testreg")
			Expect(yaml.Unmarshal([]byte(cmYaml), &cm)).Should(BeNil())
			Expect(k8sClient.Update(ctx, &cm)).Should(BeNil())
		})

		AfterEach(func() {
			server.Close()
		})

		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{}, []string{})
				Expect(err).To(HaveOccurred())
			})
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, nil, []string{"testreg"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("one existing addon name provided", func() {
			It("should return one valid result, matching all registries", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"example"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
			It("should return one valid result, matching one registry", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"example"}, []string{"testreg"})
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
		})

		Context("one non-existent addon name provided", func() {
			It("should return error as ErrNotExist", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"non-existent-addon"}, nil)
				Expect(errors.Is(err, ErrNotExist)).To(BeTrue())
				Expect(res).To(BeNil())
			})
		})

		Context("one existing addon name and one non-existent addon name provided", func() {
			It("should return only one valid result", func() {
				res, err := FindWholeAddonPackagesFromRegistry(context.Background(), k8sClient, []string{"example", "non-existent-addon"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
		})
	})
})
