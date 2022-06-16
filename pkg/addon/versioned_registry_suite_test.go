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

package addon

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test Versioned Registry", func() {
	versionedRegistryHttpHandler := &http.ServeMux{}

	versionedRegistryHttpHandler.HandleFunc("/", versionedHandler)
	versionedRegistryHttpHandler.HandleFunc("/authReg", basicAuthVersionedHandler)
	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", 18083), versionedRegistryHttpHandler)
		Expect(err).ShouldNot(HaveOccurred())
	}()
	go func() {
		err := http.ListenAndServeTLS(fmt.Sprintf(":%d", 18443),
			"./testdata/tls/local-selfsign.crt", "./testdata/tls/local-selfsign.key", versionedRegistryHttpHandler)
		Expect(err).ShouldNot(HaveOccurred())
	}()

	time.Sleep(3 * time.Second)

	registries := []Registry{
		{
			Name: "helm-repo",
			Helm: &HelmSource{URL: "http://127.0.0.1:18083"},
		},
		{
			Name: "auth-helm-repo",
			Helm: &HelmSource{
				URL:      "http://127.0.0.1:18083",
				Username: "kubevela",
				Password: "versioned registry",
			},
		},
		{
			Name: "tls-helm-repo",
			Helm: &HelmSource{URL: "https://127.0.0.1:18443", InsecureSkipTLS: true},
		},
		{
			Name: "auth-tls-helm-repo",
			Helm: &HelmSource{
				URL:             "https://127.0.0.1:18443",
				Username:        "kubevela",
				Password:        "versioned registry",
				InsecureSkipTLS: true,
			},
		},
	}

	for _, registry := range registries {
		registry := registry
		r := BuildVersionedRegistry(registry.Name, registry.Helm.URL, &common.HTTPOption{
			InsecureSkipTLS: registry.Helm.InsecureSkipTLS,
			Username:        registry.Helm.Username,
			Password:        registry.Helm.Password,
		})
		Context(fmt.Sprintf("Test %s", registry.Name), func() {
			It("list addon", func() {
				addon, err := r.ListAddon()
				Expect(err).NotTo(HaveOccurred())
				Expect(addon).To(HaveLen(1))
				Expect(addon[0].Name).To(Equal("fluxcd"))
				Expect(addon[0].AvailableVersions).To(HaveLen(1))
			})

			It("get addon ui data", func() {
				addonUIData, err := r.GetAddonUIData(context.Background(), "fluxcd", "1.0.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(addonUIData).NotTo(BeNil())
				Expect(addonUIData.Definitions).NotTo(BeEmpty())
				Expect(addonUIData.Icon).NotTo(BeEmpty())
			})

			It("get addon install pkg", func() {
				addonsInstallPackage, err := r.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(addonsInstallPackage).NotTo(BeNil())
				Expect(addonsInstallPackage.YAMLTemplates).NotTo(BeEmpty())
				Expect(addonsInstallPackage.DefSchemas).NotTo(BeEmpty())
			})

			It("get addon defail", func() {
				addonWholePackage, err := r.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
				Expect(err).NotTo(HaveOccurred())
				Expect(addonWholePackage).NotTo(BeNil())
				Expect(addonWholePackage.YAMLTemplates).NotTo(BeEmpty())
				Expect(addonWholePackage.DefSchemas).NotTo(BeEmpty())
				Expect(addonWholePackage.RegistryName).NotTo(BeEmpty())
			})
		})
	}

	errRegistries := []Registry{
		{
			Name: "tls-err-helm-repo",
			Helm: &HelmSource{URL: "https://127.0.0.1:18443", InsecureSkipTLS: false},
		},
		{
			Name: "auth-tls-err-helm-repo",
			Helm: &HelmSource{
				URL:             "https://127.0.0.1:18443",
				Username:        "kubevela",
				Password:        "versioned registry",
				InsecureSkipTLS: false,
			},
		},
	}

	for _, registry := range errRegistries {
		registry := registry
		r := BuildVersionedRegistry(registry.Name, registry.Helm.URL, &common.HTTPOption{
			InsecureSkipTLS: registry.Helm.InsecureSkipTLS,
			Username:        registry.Helm.Username,
			Password:        registry.Helm.Password,
		})
		Context(fmt.Sprintf("Test %s", registry.Name), func() {
			It("list addon", func() {
				addon, err := r.ListAddon()
				Expect(err).To(HaveOccurred())
				Expect(addon).To(BeEmpty())
			})

			It("get addon ui data", func() {
				addonUIData, err := r.GetAddonUIData(context.Background(), "fluxcd", "1.0.0")
				Expect(err).To(HaveOccurred())
				Expect(addonUIData).To(BeNil())
			})

			It("get addon install pkg", func() {
				addonsInstallPackage, err := r.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
				Expect(err).To(HaveOccurred())
				Expect(addonsInstallPackage).To(BeNil())
			})

			It("get addon defail", func() {
				addonWholePackage, err := r.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
				Expect(err).To(HaveOccurred())
				Expect(addonWholePackage).To(BeNil())
			})
		})
	}
})
