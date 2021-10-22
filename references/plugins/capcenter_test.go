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
	"testing"

	"github.com/oam-dev/kubevela/pkg/utils"

	"github.com/stretchr/testify/assert"
)

func TestParseURL(t *testing.T) {
	cases := map[string]struct {
		url     string
		exp     *utils.GithubContent
		expType string
	}{
		"api-github": {
			url:     "https://api.github.com/repos/zzxwill/catalog/contents/repository?ref=plugin",
			expType: utils.TypeGithub,
			exp: &utils.GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
				Ref:   "plugin",
			},
		},
		"github-copy-path": {
			url:     "https://github.com/zzxwill/catalog/tree/plugin/repository",
			expType: utils.TypeGithub,
			exp: &utils.GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
				Ref:   "plugin",
			},
		},
		"github-manuel-write-path": {
			url:     "https://github.com/zzxwill/catalog/repository",
			expType: utils.TypeGithub,
			exp: &utils.GithubContent{
				Owner: "zzxwill",
				Repo:  "catalog",
				Path:  "repository",
			},
		},
	}
	for caseName, c := range cases {
		tp, content, err := utils.Parse(c.url)
		assert.NoError(t, err, caseName)
		assert.Equal(t, c.exp, &content.GithubContent, caseName)
		assert.Equal(t, c.expType, tp, caseName)
	}
}
