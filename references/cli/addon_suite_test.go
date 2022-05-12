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

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/util"

	"github.com/fatih/color"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"

	"github.com/gosuri/uitable"
)

var _ = Describe("Output of listing addons tests", func() {
	// Output of function listAddons to test
	var actualTable *uitable.Table

	// getRowsByName extracts every rows with its NAME matching name
	getRowsByName := func(name string) []*uitable.Row {
		matchedRows := []*uitable.Row{}
		for _, row := range actualTable.Rows {
			// Check column NAME(0) = name
			if row.Cells[0].Data == name {
				matchedRows = append(matchedRows, row)
			}
		}
		return matchedRows
	}

	BeforeEach(func() {
		// Prepare KubeVela registry
		reg := &pkgaddon.Registry{
			Name: "KubeVela",
			Helm: &pkgaddon.HelmSource{
				URL: "https://addons.kubevela.net",
			},
		}
		ds := pkgaddon.NewRegistryDataStore(k8sClient)
		Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
	})

	AfterEach(func() {
		// Delete KubeVela registry
		ds := pkgaddon.NewRegistryDataStore(k8sClient)
		Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
	})

	JustBeforeEach(func() {
		// Print addon list to table for later comparison
		ret, err := listAddons(context.Background(), k8sClient, "")
		Expect(err).Should(BeNil())
		actualTable = ret
	})

	When("there is no addons installed", func() {
		It("should not have any enabled addon", func() {
			Expect(actualTable.Rows).ToNot(HaveLen(0))
			for idx, row := range actualTable.Rows {
				// Skip header
				if idx == 0 {
					continue
				}
				// Check column STATUS(4) = disabled
				Expect(row.Cells[4].Data).To(Equal("disabled"))
			}
		})
	})

	When("there is locally installed addons", func() {
		BeforeEach(func() {
			// Install fluxcd locally
			fluxcd := v1beta1.Application{}
			err := yaml.Unmarshal([]byte(fluxcdYaml), &fluxcd)
			Expect(err).Should(BeNil())
			Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		})

		It("should print fluxcd addon as local", func() {
			matchedRows := getRowsByName("fluxcd")
			Expect(matchedRows).ToNot(HaveLen(0))
			// Only use first row (local first), check column REGISTRY(1) = local
			Expect(matchedRows[0].Cells[1].Data).To(Equal("local"))
			Eventually(func() error {
				matchedRows = getRowsByName("fluxcd")
				// Check column STATUS(4) = enabled
				if matchedRows[0].Cells[4].Data != "enabled" {
					return fmt.Errorf("fluxcd is not enabled yet")
				}
				// Check column AVAILABLE-VERSIONS(3) = 1.1.0
				if versionString := matchedRows[0].Cells[3].Data; versionString != fmt.Sprintf("[%s]", color.New(color.Bold, color.FgGreen).Sprintf("1.1.0")) {
					return fmt.Errorf("fluxcd version string is incorrect: %s", versionString)
				}
				return nil
			}, 30*time.Second, 1000*time.Millisecond).Should(BeNil())
		})

		It("should print fluxcd in the registry as disabled", func() {
			matchedRows := getRowsByName("fluxcd")
			// There should be a local one and a registry one
			Expect(len(matchedRows)).To(Equal(2))
			// The registry one should be disabled
			Expect(matchedRows[1].Cells[1].Data).To(Equal("KubeVela"))
			Expect(matchedRows[1].Cells[4].Data).To(Equal("disabled"))
		})
	})
})

var _ = Describe("Addon status or info", func() {

	When("addon is not installed locally, also not in registry", func() {
		It("should only display addon name and disabled status, nothing more", func() {
			addonName := "some-nonexistent-addon"
			_, res, err := generateAddonInfo(k8sClient, addonName)
			// This is expected. Even with nonexistent addon, upstream services will return nil.
			// We will check nonexistent addon ourselves.
			Expect(err).Should(BeNil())
			expectedResponse := color.New(color.Bold).Sprintf("%s", addonName) + ": " +
				color.New(color.Faint).Sprintf("%s", statusDisabled) + " \n"
			Expect(res).To(Equal(expectedResponse))
		})
	})

	When("addon is not installed locally, but in registry", func() {
		// Prepare KubeVela registry
		BeforeEach(func() {
			reg := &pkgaddon.Registry{
				Name: "KubeVela",
				Helm: &pkgaddon.HelmSource{
					URL: "https://addons.kubevela.net",
				},
			}
			ds := pkgaddon.NewRegistryDataStore(k8sClient)
			Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
		})

		AfterEach(func() {
			// Delete KubeVela registry
			ds := pkgaddon.NewRegistryDataStore(k8sClient)
			Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
		})

		It("should display addon name and disabled status, registry name, available versions, dependencies, and parameters(optional)", func() {
			addonName := "velaux"
			_, res, err := generateAddonInfo(k8sClient, addonName)
			Expect(err).Should(BeNil())
			// Should include disabled status, like:
			// velaux: disabled
			Expect(res).To(ContainSubstring(
				color.New(color.Bold).Sprintf("%s", addonName) + ": " + color.New(color.Faint).Sprintf("%s", statusDisabled),
			))
			// Should include registry name, like:
			// ==> Registry Name
			// KubeVela
			Expect(res).To(ContainSubstring(
				color.New(color.Bold).Sprintf("%s", "Registry Name") + "\n" +
					"KubeVela",
			))
			// Should include available versions, like:
			// ==> Available Versions
			// [v2.6.3]
			Expect(res).To(ContainSubstring(
				color.New(color.Bold).Sprintf("%s", "vailable Versions") + "\n" +
					"[",
			))
			// Should include dependencies, like:
			// ==> Dependencies ✔
			// []
			Expect(res).To(ContainSubstring(
				color.New(color.Bold).Sprintf("%s", "Dependencies ") + color.GreenString("✔") + "\n" +
					"[]",
			))
			// Should include parameters, like:
			// ==> Parameters
			// -> serviceAccountName: Specify the serviceAccountName for apiserver
			Expect(res).To(ContainSubstring(
				color.New(color.Bold).Sprintf("%s", "Parameters") + "\n" +
					color.New(color.FgCyan).Sprintf("-> "),
			))
		})
	})

	When("addon is installed locally, and also in registry", func() {
		fluxcd := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(fluxcdRemoteYaml), &fluxcd)
		Expect(err).Should(BeNil())

		BeforeEach(func() {
			// Prepare KubeVela registry
			reg := &pkgaddon.Registry{
				Name: "KubeVela",
				Helm: &pkgaddon.HelmSource{
					URL: "https://addons.kubevela.net",
				},
			}
			ds := pkgaddon.NewRegistryDataStore(k8sClient)
			Expect(ds.AddRegistry(context.Background(), *reg)).To(Succeed())
		})

		AfterEach(func() {
			// Delete KubeVela registry
			ds := pkgaddon.NewRegistryDataStore(k8sClient)
			Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
			// Delete fluxcd
			Expect(k8sClient.Delete(context.Background(), &fluxcd)).To(Succeed())
		})

		JustBeforeEach(func() {
			// Install fluxcd locally
			Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		})

		It("should display addon name and enabled status, installed clusters, registry name, available versions, dependencies, and parameters(optional)", func() {
			addonName := "fluxcd"
			Eventually(func() error {
				_, res, err := generateAddonInfo(k8sClient, addonName)
				if err != nil {
					return err
				}

				// Should include enabled status, like:
				// fluxcd: enabled (1.1.0)
				if !strings.Contains(res,
					color.New(color.Bold).Sprintf("%s", addonName),
				) {
					return fmt.Errorf("addon name incorrect, %s", res)
				}

				// We cannot really get installed clusters in test environment.
				// Might change how this test is conducted in the future.

				// Should include registry name, like:
				// ==> Registry Name
				// KubeVela
				if !strings.Contains(res,
					color.New(color.Bold).Sprintf("%s", "Registry Name")+"\n"+
						"KubeVela",
				) {
					return fmt.Errorf("registry name incorrect, %s", res)
				}

				// Should include available versions, like:
				// ==> Available Versions
				// [v2.6.3]
				if !strings.Contains(res,
					color.New(color.Bold).Sprintf("%s", "Available Versions")+"\n"+
						"[",
				) {
					return fmt.Errorf("available versions incorrect, %s", res)
				}

				// Should include dependencies, like:
				// ==> Dependencies ✔
				// []
				if !strings.Contains(res,
					color.New(color.Bold).Sprintf("%s", "Dependencies ")+color.GreenString("✔")+"\n"+
						"[]",
				) {
					return fmt.Errorf("dependencies incorrect, %s", res)
				}

				// fluxcd does not have any parameters, so we skip it.
				return nil
			}, 120*time.Second, 1000*time.Millisecond).Should(BeNil())
		})
	})

	When("addon is installed locally, but not in registry", func() {
		fluxcd := v1beta1.Application{}
		err := yaml.Unmarshal([]byte(fluxcdRemoteYaml), &fluxcd)
		Expect(err).Should(BeNil())

		BeforeEach(func() {
			// Delete KubeVela registry
			ds := pkgaddon.NewRegistryDataStore(k8sClient)
			Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
			// Install fluxcd locally
			Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
		})

		AfterEach(func() {
			// Delete fluxcd
			Expect(k8sClient.Delete(context.Background(), &fluxcd)).To(Succeed())
		})

		It("should display addon name and enabled status, installed clusters, and registry name as local, nothing more", func() {
			addonName := "fluxcd"

			Eventually(func() error {
				_, res, err := generateAddonInfo(k8sClient, addonName)
				if err != nil {
					return err
				}

				// Should include enabled status, like:
				// fluxcd: enabled (1.1.0)
				if !strings.Contains(res,
					color.New(color.Bold).Sprintf("%s", addonName)+": ",
				) {
					return fmt.Errorf("addon name and enabled status incorrect")
				}

				return nil
			}, 120*time.Second, 1000*time.Millisecond).Should(BeNil())
		})
	})
})
