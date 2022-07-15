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
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

var _ = Describe("Test Versioned Registry", func() {
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

			It("get addon detail", func() {
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

			It("get addon detail", func() {
				addonWholePackage, err := r.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
				Expect(err).To(HaveOccurred())
				Expect(addonWholePackage).To(BeNil())
			})
		})
	}

	Context("Test multiversion helm repo", func() {
		const repoName = "multiversion-helm-repo"
		mr := BuildVersionedRegistry(repoName, "http://127.0.0.1:18083/multi", nil)
		It("list addon", func() {
			addons, err := mr.ListAddon()
			Expect(err).To(Succeed())
			Expect(addons).To(HaveLen(2))
		})

		It("get addon ui data", func() {
			addonUIData, err := mr.GetAddonUIData(context.Background(), "fluxcd", "2.0.0")
			Expect(err).To(Succeed())
			Expect(addonUIData.Definitions).NotTo(BeEmpty())
			Expect(addonUIData.Icon).NotTo(BeEmpty())
			Expect(addonUIData.Version).To(Equal("2.0.0"))
		})

		It("get addon install pkg", func() {
			addonsInstallPackage, err := mr.GetAddonInstallPackage(context.Background(), "fluxcd", "1.0.0")
			Expect(err).To(Succeed())
			Expect(addonsInstallPackage).NotTo(BeNil())
			Expect(addonsInstallPackage.YAMLTemplates).NotTo(BeEmpty())
			Expect(addonsInstallPackage.DefSchemas).NotTo(BeEmpty())
			Expect(addonsInstallPackage.SystemRequirements.VelaVersion).To(HaveSuffix("1.3.0"))
			Expect(addonsInstallPackage.SystemRequirements.KubernetesVersion).To(HaveSuffix("1.10.0"))
		})

		It("get addon detail", func() {
			addonWholePackage, err := mr.GetDetailedAddon(context.Background(), "fluxcd", "1.0.0")
			Expect(err).To(Succeed())
			Expect(addonWholePackage).NotTo(BeNil())
			Expect(addonWholePackage.YAMLTemplates).NotTo(BeEmpty())
			Expect(addonWholePackage.DefSchemas).NotTo(BeEmpty())
			Expect(addonWholePackage.RegistryName).To(Equal(repoName))
			Expect(addonWholePackage.SystemRequirements.VelaVersion).To(Equal(">=1.3.0"))
			Expect(addonWholePackage.SystemRequirements.KubernetesVersion).To(Equal(">=1.10.0"))
		})

		It("get addon available version", func() {
			version, err := mr.GetAddonAvailableVersion("fluxcd")
			Expect(err).To(Succeed())
			Expect(version).To(HaveLen(2))
		})
	})
})

func stepHelmHttpServer() error {
	handler := &http.ServeMux{}
	handler.HandleFunc("/", versionedHandler)
	handler.HandleFunc("/authReg", basicAuthVersionedHandler)
	handler.HandleFunc("/multi/", multiVersionHandler)

	helmRepoHttpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", 18083),
		Handler: handler,
	}
	helmRepoHttpsServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", 18443),
		Handler: handler,
	}

	go func() {
		err := helmRepoHttpsServer.ListenAndServeTLS("./testdata/tls/local-selfsign.crt", "./testdata/tls/local-selfsign.key")
		Expect(err).ShouldNot(HaveOccurred())
	}()
	go func() {
		err := helmRepoHttpServer.ListenAndServe()
		Expect(err).ShouldNot(HaveOccurred())
	}()

	err := checkHelmHttpServer("http://127.0.0.1:18083", 3, time.Second)
	if err != nil {
		return err
	}
	err = checkHelmHttpServer("http://127.0.0.1:18443", 3, time.Second)
	if err != nil {
		return err
	}
	return nil
}

func checkHelmHttpServer(url string, maxTryNum int, interval time.Duration) error {
	client := http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	var err error
	for cur := 0; cur < maxTryNum; cur++ {
		_, err = client.Get(url)
		if err != nil {
			time.Sleep(interval)
			continue
		}
	}
	if err != nil {
		return errors.Wrap(err, "exceeded maximum number of retries.")
	}
	return nil
}
