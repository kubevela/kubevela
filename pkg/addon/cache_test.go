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
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
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
