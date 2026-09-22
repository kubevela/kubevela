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
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/go-github/v32/github"
	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/oauth2"

	"github.com/oam-dev/kubevela/pkg/registry/component"
	"github.com/oam-dev/kubevela/pkg/utils"
)

// The Git-family registry transport lives here rather than in
// pkg/registry/component, which is where it sat between commits 353296064 and
// this one. Only `vela addon enable` reads an addon from Git: a type: addon or
// type: module component resolves from an OCI registry or a Helm repository,
// and refuses a Git registry. Keeping the GitHub, Gitee and GitLab clients in
// the package that needs them leaves the shared registry layer free of them.
//
// component.NewAsyncReader delegates the three Git reader types here through
// the builder registered below, because the dependency only works in this
// direction: pkg/addon imports pkg/registry/component.

func init() {
	component.RegisterGitReaderBuilder(buildGitReader)
}

// buildGitReader is the component.GitReaderBuilder for the three Git-family
// source types. It is the body of what NewAsyncReader's git, gitee and gitlab
// cases used to do inline.
func buildGitReader(baseURL, repo, subPath, token string, rdType component.ReaderType) (component.AsyncReader, error) {
	baseURL = strings.TrimSuffix(baseURL, ".git")
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("addon registry invalid")
	}

	switch rdType {
	case gitType:
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		return &gitReader{h: createGitHelper(content, token)}, nil
	case giteeType:
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		return &giteeReader{h: createGiteeHelper(content, token)}, nil
	case gitlabType:
		_, content, err := utils.ParseGitlab(u.String(), repo)
		if err != nil {
			return nil, err
		}
		content.GitlabContent.Path = subPath
		helper, err := createGitlabHelper(content, token)
		if err != nil {
			return nil, errors.New("addon registry connect fail")
		}
		if err := helper.getGitlabProject(content); err != nil {
			return nil, err
		}
		return &gitlabReader{h: helper}, nil
	}
	return nil, fmt.Errorf("invalid addon registry type '%s'", rdType)
}

func createGitHelper(content *utils.Content, token string) *gitHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := github.NewClient(tc)
	return &gitHelper{
		Client: cli,
		Meta:   content,
	}
}

func createGiteeHelper(content *utils.Content, token string) *giteeHelper {
	var ts oauth2.TokenSource
	if token != "" {
		ts = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	}
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = time.Second * 20
	cli := NewGiteeClient(tc, nil)
	return &giteeHelper{
		Client: cli,
		Meta:   content,
	}
}

func createGitlabHelper(content *utils.Content, token string) (*gitlabHelper, error) {
	newClient, err := gitlab.NewClient(token, gitlab.WithBaseURL(content.GitlabContent.Host))

	return &gitlabHelper{
		Client: newClient,
		Meta:   content,
	}, err
}

func (h *gitlabHelper) getGitlabProject(content *utils.Content) error {
	projectURL := content.GitlabContent.Owner + "/" + content.GitlabContent.Repo
	projects, _, err := h.Client.Projects.GetProject(projectURL, &gitlab.GetProjectOptions{})
	if err != nil {
		return err
	}
	content.GitlabContent.PId = projects.ID

	return nil
}
