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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/repo"
)

func TestPutVersionedUIData2cache(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	u := NewCache(nil)
	u.putVersionedUIData2Cache("helm-repo", "fluxcd", "1.0.0", &uiData)
	assert.NotEmpty(t, u.versionedUIData)
	assert.NotEmpty(t, u.versionedUIData["helm-repo"])
	assert.NotEmpty(t, u.versionedUIData["helm-repo"]["fluxcd-1.0.0"])
	assert.Equal(t, u.versionedUIData["helm-repo"]["fluxcd-1.0.0"].Name, "fluxcd")
}

func TestPutAddonUIData2Cache(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	addons := []*UIData{&uiData}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonUIData2Cache(name, addons)
	assert.NotEmpty(t, u.uiData)
	assert.Equal(t, u.uiData[name], addons)
}

func TestListCachedUIData(t *testing.T) {
	uiData := UIData{Meta: Meta{Name: "fluxcd", Icon: "test.com/fluxcd.png", Version: "1.0.0"}}
	addons := []*UIData{&uiData}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonUIData2Cache(name, addons)

	assert.Equal(t, u.listCachedUIData(name), addons)
}

var _ = Describe("Test addon cache", func() {
	vr := Registry{Name: "helm-repo", Helm: &HelmSource{URL: "http://127.0.0.1:18083/authReg", Username: "hello", Password: "hello"}}

	It("Test list addon helm repo UI data", func() {
		uiData := UIData{Meta: Meta{
			Name:        "fluxcd",
			Description: "Extended workload to do continuous and progressive delivery",
			Icon:        "https://raw.githubusercontent.com/fluxcd/flux/master/docs/_files/weave-flux.png",
			Version:     "1.0.0",
			Tags:        []string{"extended_workload", "gitops"},
		},
			AvailableVersions: []string{"1.0.0"},
			RegistryName:      "helm-repo"}
		addons := []*UIData{&uiData}
		u := NewCache(nil)
		uiDatas, err := u.ListUIData(vr)
		Expect(err).NotTo(HaveOccurred())
		Expect(uiDatas).To(Equal(addons))
	})
})

func TestListVersionRegistryCachedUIData(t *testing.T) {
	name := "fluxcd"
	version := "v1.0.1"
	uiData := &UIData{Meta: Meta{Name: name, Icon: "test.com/fluxcd.png", Version: version}}
	addons := []*UIData{uiData}
	vrName := "helm-repo"
	u := NewCache(nil)
	u.putVersionedUIData2Cache(vrName, name, version, uiData)
	u.putVersionedUIData2Cache(vrName, name, "latest", uiData)

	assert.Equal(t, u.listVersionRegistryCachedUIData(vrName), addons)
}

func TestPutAddonMeta2Cache(t *testing.T) {
	addonMeta := map[string]SourceMeta{
		"fluxcd": {
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/definitions/helm-release.yaml",
					name: "helm-release.yaml",
				},
			},
		},
	}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonMeta2Cache(name, addonMeta)
	assert.NotEmpty(t, u.registryMeta)
	assert.Equal(t, u.registryMeta[name], addonMeta)
}

func TestGetCachedAddonMeta(t *testing.T) {
	addonMeta := map[string]SourceMeta{
		"fluxcd": {
			Name: "fluxcd",
			Items: []Item{
				&OSSItem{
					tp:   FileType,
					path: "fluxcd/definitions/helm-release.yaml",
					name: "helm-release.yaml",
				},
			},
		},
	}
	name := "helm-repo"
	u := NewCache(nil)
	u.putAddonMeta2Cache(name, addonMeta)

	assert.Equal(t, u.getCachedAddonMeta(name), addonMeta)
}

// registryStub serves a fixed listing and per-version data, standing in for a
// versioned registry without any network.
type registryStub struct {
	list    []*UIData
	perCall func(name, version string) *UIData
}

func (s *registryStub) ListAddon() ([]*UIData, error) { return s.list, nil }
func (s *registryStub) GetAddonUIData(_ context.Context, name, version string) (*UIData, error) {
	return s.perCall(name, version), nil
}
func (s *registryStub) GetAddonInstallPackage(context.Context, string, string) (*InstallPackage, error) {
	return nil, nil
}
func (s *registryStub) GetDetailedAddon(context.Context, string, string) (*WholeAddonPackage, error) {
	return nil, nil
}
func (s *registryStub) GetAddonAvailableVersion(string) ([]*repo.ChartVersion, error) {
	return nil, nil
}

// TestCacheVersionedUIDataKeepsAvailableVersions covers what the UI reads to
// offer version choices. The listing carries every version, but the per-version
// fetch it drives is pinned and need not, so caching the fetch result verbatim
// showed the addon as having a single version.
func TestCacheVersionedUIDataKeepsAvailableVersions(t *testing.T) {
	u := NewCache(nil)
	stub := &registryStub{
		list: []*UIData{{
			Meta:              Meta{Name: "fluxcd", Version: "3.0.2"},
			AvailableVersions: []string{"3.0.2", "3.0.1", "2.0.0"},
		}},
		perCall: func(name, version string) *UIData {
			// A pinned fetch: no version list, matching ociRegistry.loadAddon.
			return &UIData{Meta: Meta{Name: name, Version: version}}
		},
	}

	u.cacheVersionedUIData("ecr", stub, stub.list)

	cached := u.getCachedUIData(Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://reg/addon"}}, "fluxcd", "3.0.2")
	require.NotNil(t, cached)
	assert.Equal(t, []string{"3.0.2", "3.0.1", "2.0.0"}, cached.AvailableVersions)

	latest := u.getCachedUIData(Registry{Name: "ecr", OCI: &OCIAddonSource{URL: "oci://reg/addon"}}, "fluxcd", "")
	require.NotNil(t, latest)
	assert.Equal(t, []string{"3.0.2", "3.0.1", "2.0.0"}, latest.AvailableVersions)
}
