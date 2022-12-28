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
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/helm/pkg/tlsutil"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	pkgaddon "github.com/oam-dev/kubevela/pkg/addon"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
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
				Expect(row.Cells[4].Data).To(Equal("-"))
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
			Expect(matchedRows[1].Cells[4].Data).To(Equal("-"))
		})
	})
})

var _ = Describe("Addon status or info", func() {

	Context("when verbose is enabled", func() {
		BeforeEach(func() {
			verboseStatus = true
		})

		When("addon is not installed locally, also not in registry", func() {
			It("should return an error, saying not found", func() {
				addonName := "some-nonexistent-addon"
				_, _, err := generateAddonInfo(k8sClient, addonName)
				Expect(err).ShouldNot(BeNil())
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
				res, _, err := generateAddonInfo(k8sClient, addonName)
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
				// Delete fluxcd
				Expect(k8sClient.Delete(context.Background(), &fluxcd)).To(Succeed())
				// Delete KubeVela registry
				ds := pkgaddon.NewRegistryDataStore(k8sClient)
				Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).To(Succeed())
			})

			JustBeforeEach(func() {
				// Install fluxcd locally
				Expect(k8sClient.Create(context.Background(), &fluxcd)).Should(SatisfyAny(BeNil(), util.AlreadyExistMatcher{}))
			})

			It("should display addon name and enabled status, installed clusters, registry name, available versions, dependencies, and parameters(optional)", func() {
				addonName := "fluxcd"
				Eventually(func() error {
					res, _, err := generateAddonInfo(k8sClient, addonName)
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

					// We cannot really get installed clusters in this test environment.
					// Might change how this test is conducted in the future.
					return nil
				}, 30*time.Second, 1000*time.Millisecond).Should(BeNil())
			})
		})

		When("addon is installed locally, but not in registry", func() {
			fluxcd := v1beta1.Application{}
			err := yaml.Unmarshal([]byte(fluxcdYaml), &fluxcd)
			Expect(err).Should(BeNil())

			BeforeEach(func() {
				// Delete KubeVela registry
				ds := pkgaddon.NewRegistryDataStore(k8sClient)
				Expect(ds.DeleteRegistry(context.Background(), "KubeVela")).Should(SatisfyAny(Succeed(), util.NotFoundMatcher{}))
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
					res, _, err := generateAddonInfo(k8sClient, addonName)
					if err != nil {
						return err
					}
					fmt.Println(addonName, res, err)
					// Should include enabled status, like:
					// fluxcd: enabled (1.1.0)
					if !strings.Contains(res,
						color.New(color.Bold).Sprintf("%s", addonName)+": ",
					) {
						return fmt.Errorf("addon name and enabled status incorrect:, %s", res)
					}
					// We cannot really get installed clusters in this test environment.
					// Might change how this test is conducted in the future.

					// Should include registry name, like:
					// ==> Registry Name
					// local
					if !strings.Contains(res,
						color.New(color.Bold).Sprintf("%s", "Registry Name")+"\n"+
							"local",
					) {
						return fmt.Errorf("registry name incorrect, %s", res)
					}
					return nil
				}, 30*time.Second, 1000*time.Millisecond).Should(BeNil())
			})
		})
	})

	Context("when verbose is disabled", func() {
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

			It("should display addon name and disabled status, and registry name", func() {
				addonName := "dex"
				res, _, err := generateAddonInfo(k8sClient, addonName)
				Expect(err).Should(BeNil())
				// Should include disabled status, like:
				// dex: disabled
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
			})
			It("should report addon not exist in any registry name", func() {
				addonName := "not-exist"
				_, _, err := generateAddonInfo(k8sClient, addonName)
				Expect(err.Error()).Should(BeEquivalentTo("addon 'not-exist' not found in cluster or any registry"))
			})
		})
	})
})

var _ = Describe("Addon push command", func() {
	var c common.Args
	var (
		testTarballPath    = "../../pkg/addon/testdata/charts/sample-1.0.1.tgz"
		testServerCertPath = "../../pkg/addon/testdata/tls/server.crt"
		testServerKeyPath  = "../../pkg/addon/testdata/tls/server.key"
		testServerCAPath   = "../../pkg/addon/testdata/tls/server_ca.crt"
		testClientCAPath   = "../../pkg/addon/testdata/tls/client_ca.crt"
		testClientCertPath = "../../pkg/addon/testdata/tls/client.crt"
		testClientKeyPath  = "../../pkg/addon/testdata/tls/client.key"
	)
	var (
		statusCode int
		body       string
		tmp        string
	)
	var ts *httptest.Server
	var err error

	AfterEach(func() {
		ts.Close()
		_ = os.RemoveAll(tmp)
		err = deleteAddonRegistry(context.TODO(), c, "helm-push-test")
		Expect(err).To(Succeed())
	})

	Context("plain old HTTP Server", func() {
		BeforeEach(func() {
			c.SetClient(k8sClient)
			c.SetConfig(cfg)

			statusCode = 201
			body = "{\"success\": true}"
			ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
				w.Write([]byte(body))
			}))

			// Create new Helm home w/ test repo
			tmp, err = os.MkdirTemp("", "helm-push-test")
			Expect(err).To(Succeed())

			// Add our helm repo to addon registry
			err = addAddonRegistry(context.TODO(), c, pkgaddon.Registry{
				Name: "helm-push-test",
				Helm: &pkgaddon.HelmSource{
					URL: ts.URL,
				},
			})
			Expect(err).To(Succeed())

			_ = os.Setenv("HELM_REPO_USERNAME", "myuser")
			_ = os.Setenv("HELM_REPO_PASSWORD", "mypass")
			_ = os.Setenv("HELM_REPO_CONTEXT_PATH", "/x/y/z")
		})

		It("Not enough args", func() {
			args := []string{}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expecting error with missing args, instead got nil")
		})

		It("Bad chart path", func() {
			args := []string{"/this/this/not/a/chart", "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad chart path, instead got nil")
		})

		It("Bad repo name", func() {
			args := []string{testTarballPath, "this-is-not-a-valid-repo"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad repo name, instead got nil")
		})

		It("Valid tar, repo name", func() {
			args := []string{testTarballPath, "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).Should(Succeed())
		})

		It("Valid tar, repo URL", func() {
			args := []string{testTarballPath, ts.URL}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).Should(Succeed())
		})

		It("Trigger 409, already exists", func() {
			statusCode = 409
			body = "{\"error\": \"package already exists\"}"
			args := []string{testTarballPath, "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expecting error with 409, instead got nil")
		})

		It("Unable to parse JSON response body", func() {
			statusCode = 500
			body = "duiasnhioasd"
			args := []string{testTarballPath, "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expecting error with bad response body, instead got nil")
		})
	})

	Context("TLS Enabled Server", func() {
		BeforeEach(func() {
			c.SetClient(k8sClient)
			c.SetConfig(cfg)

			statusCode = 201
			body = "{\"success\": true}"
			ts = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(statusCode)
				_, _ = w.Write([]byte(body))
			}))
			serverCert, err := tls.LoadX509KeyPair(testServerCertPath, testServerKeyPath)
			Expect(err).To(Succeed(), "failed to load certificate and key")

			clientCaCertPool, err := tlsutil.CertPoolFromFile(testClientCAPath)
			Expect(err).To(Succeed(), "load server CA file failed")

			ts.TLS = &tls.Config{
				ClientCAs:    clientCaCertPool,
				ClientAuth:   tls.RequireAndVerifyClientCert,
				Certificates: []tls.Certificate{serverCert},
				Rand:         rand.Reader,
			}
			ts.StartTLS()

			// Create new Helm home w/ test repo
			tmp, err = os.MkdirTemp("", "helm-push-test")
			Expect(err).To(Succeed())

			// Add our helm repo to addon registry
			err = addAddonRegistry(context.TODO(), c, pkgaddon.Registry{
				Name: "helm-push-test",
				Helm: &pkgaddon.HelmSource{
					URL: ts.URL,
				},
			})
			Expect(err).To(Succeed())

			_ = os.Setenv("HELM_REPO_USERNAME", "myuser")
			_ = os.Setenv("HELM_REPO_PASSWORD", "mypass")
			_ = os.Setenv("HELM_REPO_CONTEXT_PATH", "/x/y/z")
		})

		It("no cert provided", func() {
			_ = os.Unsetenv("HELM_REPO_CA_FILE")
			_ = os.Unsetenv("HELM_REPO_CERT_FILE")
			_ = os.Unsetenv("HELM_REPO_KEY_FILE")
			args := []string{testTarballPath, "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).ShouldNot(Succeed(), "expected non nil error but got nil when run cmd without certificate option")
		})

		It("with cert", func() {
			_ = os.Setenv("HELM_REPO_CA_FILE", testServerCAPath)
			_ = os.Setenv("HELM_REPO_CERT_FILE", testClientCertPath)
			_ = os.Setenv("HELM_REPO_KEY_FILE", testClientKeyPath)
			args := []string{testTarballPath, "helm-push-test"}
			cmd := NewAddonPushCommand(c)
			cmd.SetArgs(args)
			err := cmd.RunE(cmd, args)
			Expect(err).Should(Succeed())
		})
	})
})
