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
