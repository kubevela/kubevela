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
	"path"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/utils"
)

var _ AsyncReader = &gitReader{}

// gitHelper helps get addon's file by git
type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

type gitReader struct {
	h *gitHelper
}

// ListAddonMeta relative path to repoURL/basePath
func (g *gitReader) ListAddonMeta() (map[string]SourceMeta, error) {
	subItems := make(map[string]SourceMeta)
	_, items, err := g.h.readRepo("")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// single addon
		addonName := path.Base(item.GetPath())
		addonMeta, err := g.listAddonMeta(g.RelativePath(item))
		if err != nil {
			return nil, errors.Wrapf(err, "fail to get addon meta of %s", addonName)
		}
		subItems[addonName] = SourceMeta{Name: addonName, Items: addonMeta}
	}
	return subItems, nil
}

func (g *gitReader) listAddonMeta(dirPath string) ([]Item, error) {
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
func (g *gitReader) ReadFile(relativePath string) (content string, err error) {
	file, _, err := g.h.readRepo(relativePath)
	if err != nil {
		return
	}
	if file == nil {
		return "", fmt.Errorf("path %s is not a file", relativePath)
	}
	return file.GetContent()
}

func (g *gitReader) RelativePath(item Item) string {
	absPath := strings.Split(item.GetPath(), "/")
	if g.h.Meta.Path == "" {
		return path.Join(absPath...)
	}
	base := strings.Split(g.h.Meta.Path, "/")
	return path.Join(absPath[len(base):]...)
}
