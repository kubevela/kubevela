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
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func setupMockServer() *httptest.Server {
	var listenURL string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fileList := []string{
			"index.yaml",
			"fluxcd-test-version-1.0.0.tgz",
			"fluxcd-test-version-2.0.0.tgz",
			"vela-workflow-v0.3.5.tgz",
			"foo-v1.0.0.tgz",
			"bar-v1.0.0.tgz",
			"bar-v2.0.0.tgz",
			"mock-be-dep-addon-v1.0.0.tgz",
		}
		for _, f := range fileList {
			if strings.Contains(req.URL.Path, f) {
				file, err := os.ReadFile("../../e2e/addon/mock/testrepo/helm-repo/" + f)
				if err != nil {
					_, _ = w.Write([]byte(err.Error()))
				}
				if f == "index.yaml" {
					// in index.yaml, url is hardcoded to 127.0.0.1:9098,
					// so we need to replace it with the real random listen url
					file = bytes.ReplaceAll(file, []byte("http://127.0.0.1:9098"), []byte(listenURL))
				}
				_, _ = w.Write(file)
			}
		}
	}))
	listenURL = s.URL
	return s
}

var _ = Describe("test FindAddonPackagesDetailFromRegistry", func() {
	Describe("when no registry is added, no matter what you do, it will just return error", func() {
		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{}, []string{})
				Expect(err).To(HaveOccurred())
			})
			It("should return error", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, nil, nil)
				Expect(err).To(HaveOccurred())
			})
		})
		Context("when non-empty addonNames and registryNames is supplied", func() {
			It("should return error saying ErrRegistryNotExist", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"fluxcd"}, []string{"some-registry"})
				Expect(errors.Is(err, ErrRegistryNotExist)).To(BeTrue())
			})
		})
	})

	Describe("one versioned registry is added", func() {
		var s *httptest.Server

		BeforeEach(func() {
			s = setupMockServer()
			// Prepare registry
			reg := &Registry{
				Name: "addon_helper_test",
				Helm: &HelmSource{
					URL: s.URL,
				},
			}
			ds := NewRegistryDataStore(k8sClient)
			Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up registry
			ds := NewRegistryDataStore(k8sClient)
			Expect(ds.DeleteRegistry(context.Background(), "addon_helper_test")).To(Succeed())
			s.Close()
		})

		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{}, []string{"addon_helper_test"})
				Expect(err).To(HaveOccurred())
			})
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, nil, []string{"addon_helper_test"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("one existing addon name provided", func() {
			It("should return one valid result, matching all registries", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"foo"}, nil)

				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("foo"))
			})
			It("should return one valid result, matching one registry", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"foo"}, []string{"addon_helper_test"})
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("foo"))
			})
		})

		Context("one non-existent addon name provided", func() {
			It("should return error as ErrNotExist", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"non-existent-addon"}, nil)
				Expect(errors.Is(err, ErrNotExist)).To(BeTrue())
				Expect(res).To(BeNil())
			})
		})

		Context("two existing addon names provided", func() {
			It("should return two valid result", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"foo", "bar"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(2))
				Expect(res[0].Name).To(Equal("foo"))
				Expect(res[1].Name).To(Equal("bar"))
			})
		})

		Context("one existing addon name and one non-existent addon name provided", func() {
			It("should return only one valid result", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"foo", "non-existent-addon"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("foo"))
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
			_ = k8sClient.Create(ctx, &cm)
			Expect(k8sClient.Update(ctx, &cm)).Should(BeNil())
		})

		AfterEach(func() {
			server.Close()
		})

		Context("when empty addonNames and registryNames is supplied", func() {
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{}, []string{})
				Expect(err).To(HaveOccurred())
			})
			It("should return error, empty addonNames are not allowed", func() {
				_, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, nil, []string{"testreg"})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("one existing addon name provided", func() {
			It("should return one valid result, matching all registries", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"example"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
			It("should return one valid result, matching one registry", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"example"}, []string{"testreg"})
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
		})

		Context("one non-existent addon name provided", func() {
			It("should return error as ErrNotExist", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"non-existent-addon"}, nil)
				Expect(errors.Is(err, ErrNotExist)).To(BeTrue())
				Expect(res).To(BeNil())
			})
		})

		Context("one existing addon name and one non-existent addon name provided", func() {
			It("should return only one valid result", func() {
				res, err := FindAddonPackagesDetailFromRegistry(context.Background(), k8sClient, []string{"example", "non-existent-addon"}, nil)
				Expect(err).To(Succeed())
				Expect(res).To(HaveLen(1))
				Expect(res[0].Name).To(Equal("example"))
				Expect(res[0].InstallPackage).ToNot(BeNil())
			})
		})
	})
})
