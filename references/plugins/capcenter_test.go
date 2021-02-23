package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		assert.Equal(t, c.exp, content, caseName)
		assert.Equal(t, c.expType, tp, caseName)
	}
}
