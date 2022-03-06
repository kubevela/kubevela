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
	"fmt"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"path"
	"strings"
)

var _ AsyncReader = &giteeReader{}

// giteeHelper helps get addon's file by git
type giteeHelper struct {
	Client *Client
	Meta   *utils.Content
}

type Client struct {
	Client  *http.Client
	BaseURL *url.URL
}

type giteeReader struct {
	h *giteeHelper
}

func NewGiteeClient(httpClient *http.Client, baseURL *url.URL) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if baseURL == nil {
		baseURL, _ = baseURL.Parse(DefaultGiteeURL)
	}
	return &Client{httpClient, baseURL}
}

// ListAddonMeta relative path to repoURL/basePath
func (g *giteeReader) ListAddonMeta() (map[string]SourceMeta, error) {
	subItems := make(map[string]SourceMeta)
	_, items, err := g.h.readRepo("")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// single addon
		if item.GetType() != DirType {
			continue
		}
		addonName := path.Base(item.GetPath())
		addonMeta, err := g.listAddonMeta(g.RelativePath(item))
		if err != nil {
			return nil, errors.Wrapf(err, "fail to get addon meta of %s", addonName)
		}
		subItems[addonName] = SourceMeta{Name: addonName, Items: addonMeta}
	}
	return subItems, nil
}

func (g *giteeReader) listAddonMeta(dirPath string) ([]Item, error) {
	_, items, err := g.h.readRepo(dirPath)
	if err != nil {
		return nil, err
	}
	res := make([]Item, 0)
	for _, item := range items {
		switch item.GetType() {
		case FileType:
			res = append(res, item)
		case DirType:
			subItems, err := g.listAddonMeta(g.RelativePath(item))
			if err != nil {
				return nil, err
			}
			res = append(res, subItems...)
		}
	}
	return res, nil
}

// ReadFile read file content from github
func (g *giteeReader) ReadFile(relativePath string) (content string, err error) {
	file, _, err := g.h.readRepo(relativePath)
	if err != nil {
		return
	}
	if file == nil {
		return "", fmt.Errorf("path %s is not a file", relativePath)
	}
	return file.GetContent()
}

func (g *giteeReader) RelativePath(item Item) string {
	absPath := strings.Split(item.GetPath(), "/")
	if g.h.Meta.GiteeContent.Path == "" {
		return path.Join(absPath...)
	}
	base := strings.Split(g.h.Meta.GiteeContent.Path, "/")
	return path.Join(absPath[len(base):]...)
}
