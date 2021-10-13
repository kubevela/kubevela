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

package plugins

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistry(t *testing.T) {
	testAddon := "dynamic-sa"
	regName := "testReg"
	localPath, err := filepath.Abs("../../e2e/plugin/testdata")
	assert.Nil(t, err)

	cases := map[string]struct {
		url       string
		expectReg Registry
	}{
		"oss registry": {
			url:       "oss://registry.kubevela.net/",
			expectReg: OssRegistry{},
		},
		"github registry": {
			url:       "https://github.com/oam-dev/catalog/tree/master/registry",
			expectReg: GithubRegistry{},
		},
		"local registry": {
			url:       "file://" + localPath,
			expectReg: LocalRegistry{},
		},
	}

	for _, c := range cases {
		registry, err := NewRegistry(context.Background(), "", regName, c.url)
		assert.NoError(t, err, c.url)
		assert.IsType(t, c.expectReg, registry, regName)

		caps, err := registry.ListCaps()
		assert.NoError(t, err, c.url)
		assert.NotEmpty(t, caps, c.url)

		capability, data, err := registry.GetCap(testAddon)
		assert.NoError(t, err, c.url)
		assert.NotNil(t, capability, testAddon)
		assert.NotNil(t, data, testAddon)
	}
}

func TestParseURL(t *testing.T) {
	cases := map[string]struct {
		url     string
		exp     *GithubContent
		expType string
	}{
		"api-github": {
			url:     "https://api.github.com/repos/zzxwill/catalog/contents/repository?ref=plugin",
			expType: TypeGithub,
			exp: &GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
				Ref:   "plugin",
			},
		},
		"github-copy-path": {
			url:     "https://github.com/zzxwill/catalog/tree/plugin/repository",
			expType: TypeGithub,
			exp: &GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
				Ref:   "plugin",
			},
		},
		"github-manuel-write-path": {
			url:     "https://github.com/zzxwill/catalog/repository",
			expType: TypeGithub,
			exp: &GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
			},
		},
	}
	for caseName, c := range cases {
		tp, content, err := Parse(c.url)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.exp, &content.GithubContent, caseName)
		assert.Equal(t, c.expType, tp, caseName)
	}
}
