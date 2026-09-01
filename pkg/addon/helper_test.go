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
	"testing"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func setupMockServer() *httptest.Server {
	var listenURL string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fileList := []string{
			"index.yaml",
			"fluxcd-test-version-1.0.0.tgz",
			"fluxcd-test-version-2.0.0.tgz",
			"vela-workflow-v0.6.2.tgz",
			"foo-v1.0.0.tgz",
			"bar-v1.0.0.tgz",
			"bar-v2.0.0.tgz",
			"mock-be-dep-addon-v1.0.0.tgz",
			"has-clusters-arg-v1.0.0.tgz",
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
			cm := corev1.ConfigMap{}
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

func TestExtractDefinitionNameFromFile(t *testing.T) {
	assert.Equal(t, "component", extractDefinitionNameFromFile(ElementFile{Name: "definitions/component.cue"}))
	assert.Equal(t, "trait", extractDefinitionNameFromFile(ElementFile{Name: "trait.yaml"}))
	assert.Equal(t, "my-policy", extractDefinitionNameFromFile(ElementFile{Name: "/tmp/my-policy.json"}))
}

func TestRemoveConflictingDefinitions(t *testing.T) {
	defs := []ElementFile{
		{Name: "definitions/comp.cue", Data: "comp"},
		{Name: "definitions/trait.cue", Data: "trait"},
		{Name: "definitions/policy.cue", Data: "policy"},
	}

	t.Run("removes all conflicting names", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, []string{"trait", "policy"})
		assert.Equal(t, []ElementFile{{Name: "definitions/comp.cue", Data: "comp"}}, filtered)
	})

	t.Run("no conflicts keeps all definitions", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, []string{"non-existent"})
		assert.Equal(t, defs, filtered)
	})

	t.Run("empty conflict list keeps all definitions", func(t *testing.T) {
		filtered := removeConflictingDefinitions(defs, nil)
		assert.Equal(t, defs, filtered)
	})
}

// TestDefinitionConflictDetectionMatchesRemoval pins the full pipeline
// DetectDefinitionConflicts -> removeConflictingDefinitions relies on: both
// stages must extract the same name for the same file, or a real conflict is
// flagged but never removed. extractDefinitionName (godef.go) and
// extractDefinitionNameFromFile (helper.go) used to disagree on directory
// prefixes, so a CUE file under "definitions/" could never collide with a
// compiled Go definition.
func TestDefinitionConflictDetectionMatchesRemoval(t *testing.T) {
	cueDefs := []ElementFile{{Name: "definitions/webservice.cue", Data: "cue-source"}}
	goDefs := []ElementFile{{Name: "component-webservice.cue", Data: "go-compiled"}}

	conflicts := DetectDefinitionConflicts(cueDefs, goDefs)
	assert.Equal(t, []string{"webservice"}, conflicts)

	filtered := removeConflictingDefinitions(cueDefs, conflicts)
	assert.Empty(t, filtered, "the definitions/-prefixed CUE file must be removed once its extracted name conflicts")
}

func TestValidateSystemRequirements(t *testing.T) {
	t.Run("nil requirement always passes without touching the cluster", func(t *testing.T) {
		assert.NoError(t, ValidateSystemRequirements(context.Background(), nil, nil, nil))
	})

	t.Run("non-nil requirement delegates to checkAddonVersionMeetRequired", func(t *testing.T) {
		listErr := errors.New("boom")
		k8sClient := &test.MockClient{MockList: test.NewMockListFn(listErr)}

		err := ValidateSystemRequirements(context.Background(), &SystemRequirements{VelaVersion: ">=1.0.0"}, k8sClient, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, listErr)
	})
}

func TestGetAddonInstallPackageFromRegistry(t *testing.T) {
	newFakeClient := func() *fake.ClientBuilder {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))
		return fake.NewClientBuilder().WithScheme(scheme)
	}

	t.Run("registry not found is wrapped with its name", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "missing-registry", "fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), `get registry "missing-registry"`)
	})

	t.Run("OCI registry resolves through the versioned OCI path and fails fast against an unreachable host", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{
			Name: "ecr",
			Helm: &HelmSource{URL: "oci://127.0.0.1:1/addon"},
		}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "ecr", "fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("Helm registry resolves through the versioned Helm path and fails fast against an unreachable host", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{
			Name: "helm-reg",
			Helm: &HelmSource{URL: "http://127.0.0.1:1"},
		}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "helm-reg", "fluxcd", "1.0.0")
		require.Error(t, err)
	})

	t.Run("registry with no source info fails building a reader for the fallback path", func(t *testing.T) {
		kubeClient := newFakeClient().Build()
		store := NewRegistryDataStore(kubeClient)
		require.NoError(t, store.AddRegistry(context.Background(), Registry{Name: "bare"}))

		_, err := GetAddonInstallPackageFromRegistry(context.Background(), kubeClient, "bare", "fluxcd", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "enough info")
	})
}

func TestCheckVersionPinSupported(t *testing.T) {
	cases := map[string]struct {
		requested string
		available string
		wantErr   bool
	}{
		"no pin is always fine":                                    {requested: "", available: "1.0.0", wantErr: false},
		"no pin and no version is fine":                            {requested: "", available: "", wantErr: false},
		"a pin matching what is served is fine":                    {requested: "1.0.0", available: "1.0.0", wantErr: false},
		"a pin the registry cannot honor is an err":                {requested: "2.0.0", available: "1.0.0", wantErr: true},
		"a pin against an unversioned addon errors":                {requested: "2.0.0", available: "", wantErr: true},
		"a v-prefixed available version matches an unprefixed pin": {requested: "1.0.0", available: "v1.0.0", wantErr: false},
		"an unprefixed available version matches a v-prefixed pin": {requested: "v1.0.0", available: "1.0.0", wantErr: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := checkVersionPinSupported("my-git-registry", "fluxcd", tc.requested, tc.available)
			if !tc.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			// The message has to name the registry, the addon and both versions, so
			// the Application status says why the pin was refused.
			assert.Contains(t, err.Error(), "my-git-registry")
			assert.Contains(t, err.Error(), "fluxcd")
			assert.Contains(t, err.Error(), tc.requested)
			assert.Contains(t, err.Error(), "does not support version pinning")
		})
	}
}
