/*
Copyright 2026 The KubeVela Authors.

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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils"
)

func TestGitReaderBuilderIsRegisteredWithTheRegistryPackage(t *testing.T) {
	// The registry package holds no Git client; linking this package is what
	// makes its Git-family reader types resolvable.
	reader, err := component.NewAsyncReader("https://github.com/kubevela/catalog", "", "", "addons", "", component.GitType)
	require.NoError(t, err)
	assert.IsType(t, &gitReader{}, reader)
}

func TestBuildGitReaderForGitHub(t *testing.T) {
	t.Run("tree URL with sub path, token and ref override", func(t *testing.T) {
		reader, err := buildGitReader("https://github.com/kubevela/catalog/tree/master", "", "addons", "tok", gitType, "v1.9")
		require.NoError(t, err)
		gh, ok := reader.(*gitReader)
		require.True(t, ok)
		require.NotNil(t, gh.h.Client)
		assert.Equal(t, utils.GithubContent{Owner: "kubevela", Repo: "catalog", Path: "addons", Ref: "v1.9"}, gh.h.Meta.GithubContent,
			"the caller's ref overrides the branch in the URL")
	})

	t.Run(".git suffix and default branch without a token", func(t *testing.T) {
		reader, err := buildGitReader("https://github.com/kubevela/catalog.git", "", "addons", "", gitType, "")
		require.NoError(t, err)
		gh := reader.(*gitReader)
		require.NotNil(t, gh.h.Client, "an anonymous client is still a client")
		assert.Equal(t, utils.GithubContent{Owner: "kubevela", Repo: "catalog", Path: "addons", Ref: ""}, gh.h.Meta.GithubContent)
	})

	t.Run("not a GitHub URL", func(t *testing.T) {
		_, err := buildGitReader("https://gitee.com/kubevela/catalog", "", "addons", "", gitType, "")
		require.Error(t, err)
		assert.Equal(t, "addon registry invalid", err.Error())
	})

	t.Run("too short to name a repository", func(t *testing.T) {
		_, err := buildGitReader("https://github.com/kubevela", "", "", "", gitType, "")
		assert.Error(t, err)
	})

	t.Run("unparsable URL", func(t *testing.T) {
		_, err := buildGitReader("://not-a-url", "", "", "", gitType, "")
		require.Error(t, err)
		assert.Equal(t, "addon registry invalid", err.Error())
	})
}

func TestBuildGitReaderForGitee(t *testing.T) {
	t.Run("tree URL with ref override", func(t *testing.T) {
		reader, err := buildGitReader("https://gitee.com/kubevela/catalog/tree/dev", "", "addons", "tok", giteeType, "release-1.9")
		require.NoError(t, err)
		ge, ok := reader.(*giteeReader)
		require.True(t, ok)
		require.NotNil(t, ge.h.Client)
		assert.Equal(t, utils.GiteeContent{Owner: "kubevela", Repo: "catalog", Path: "addons", Ref: "release-1.9"}, ge.h.Meta.GiteeContent)
	})

	t.Run("plain URL keeps the default branch", func(t *testing.T) {
		reader, err := buildGitReader("https://gitee.com/kubevela/catalog", "", "addons", "", giteeType, "")
		require.NoError(t, err)
		assert.Equal(t, utils.GiteeContent{Owner: "kubevela", Repo: "catalog", Path: "addons", Ref: ""}, reader.(*giteeReader).h.Meta.GiteeContent)
	})

	t.Run("not a Gitee URL", func(t *testing.T) {
		_, err := buildGitReader("https://github.com/kubevela/catalog", "", "addons", "", giteeType, "")
		require.Error(t, err)
		assert.Equal(t, "addon registry invalid", err.Error())
	})

	t.Run("too short to name a repository", func(t *testing.T) {
		_, err := buildGitReader("https://gitee.com/kubevela", "", "", "", giteeType, "")
		assert.Error(t, err)
	})
}

// gitlabServer answers the one GitLab API call the builder makes, the project
// lookup for owner/repo, with the given status and project id.
func gitlabServer(t *testing.T, status, projectID int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// go-gitlab escapes the "/" in owner/repo; Go unescapes it again in Path.
		if !strings.HasSuffix(r.URL.Path, "/api/v4/projects/team/addons") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": projectID, "path_with_namespace": "team/addons"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBuildGitReaderForGitLab(t *testing.T) {
	t.Run("resolves the project id on a self-hosted instance", func(t *testing.T) {
		srv := gitlabServer(t, http.StatusOK, 42)
		reader, err := buildGitReader(srv.URL+"/team/addons", "addons", "catalog", "tok", gitlabType, "main")
		require.NoError(t, err)
		gl, ok := reader.(*gitlabReader)
		require.True(t, ok)
		require.NotNil(t, gl.h.Client)
		assert.Equal(t, utils.GitlabContent{Host: srv.URL, Owner: "team", Repo: "addons", Path: "catalog", Ref: "main", PId: 42}, gl.h.Meta.GitlabContent,
			"the project id is what every later content call is keyed by")
	})

	t.Run("tree URL names the branch", func(t *testing.T) {
		srv := gitlabServer(t, http.StatusOK, 7)
		reader, err := buildGitReader(srv.URL+"/team/addons/tree/release", "addons", "", "", gitlabType, "")
		require.NoError(t, err)
		content := reader.(*gitlabReader).h.Meta.GitlabContent
		assert.Equal(t, "release", content.Ref)
		assert.Equal(t, 7, content.PId)
	})

	t.Run("project lookup fails", func(t *testing.T) {
		srv := gitlabServer(t, http.StatusNotFound, 0)
		_, err := buildGitReader(srv.URL+"/team/addons", "addons", "", "", gitlabType, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("repository name not in the URL", func(t *testing.T) {
		_, err := buildGitReader("https://gitlab.example.com/team/addons", "other", "", "", gitlabType, "")
		require.Error(t, err)
		assert.Equal(t, "addon registry repo name invalid", err.Error())
	})
}

func TestBuildGitReaderRefusesOtherSourceTypes(t *testing.T) {
	_, err := buildGitReader("https://oss.example.com", "", "", "", component.OSSType, "")
	require.Error(t, err)
	assert.Equal(t, "invalid addon registry type 'oss'", err.Error())

	_, err = buildGitReader("https://svn.example.com", "", "", "", component.ReaderType("svn"), "")
	require.Error(t, err)
	assert.Equal(t, "invalid addon registry type 'svn'", err.Error())
}

func TestCreateGitHelpersCarryTheirMeta(t *testing.T) {
	content := &utils.Content{GithubContent: utils.GithubContent{Owner: "o", Repo: "r"}}
	for _, token := range []string{"", "tok"} {
		gh := createGitHelper(content, token)
		require.NotNil(t, gh.Client)
		assert.Same(t, content, gh.Meta)

		ge := createGiteeHelper(content, token)
		require.NotNil(t, ge.Client)
		assert.Same(t, content, ge.Meta)
	}
}
