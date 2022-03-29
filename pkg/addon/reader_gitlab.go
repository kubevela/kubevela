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
	"encoding/base64"

	"github.com/xanzy/go-gitlab"

	"github.com/oam-dev/kubevela/pkg/utils"
)

var _ AsyncReader = &gitlabReader{}

// gitlabReader helps get addon's file by git
type gitlabReader struct {
	h *gitlabHelper
}

// gitlabHelper helps get addon's file by git
type gitlabHelper struct {
	Client *gitlab.Client
	Meta   *utils.Content
}

type GitLabItem struct {
	basePath string
	tp       string
	path     string
	name     string
}

func (g GitLabItem) GetType() string {
	return g.tp
}

func (g GitLabItem) GetPath() string {
	return g.path[len(g.basePath)+1:]
}

func (g GitLabItem) GetName() string {
	return g.name
}

//GetRef ref is empty , use default branch master
func (g *gitlabReader) GetRef() string {
	ref := ""
	if g.h.Meta.GitlabContent.Ref == "" {
		ref = "master"
	}
	return ref
}

//GetProjectId get gitlab project id
func (g *gitlabReader) GetProjectId() int {
	return g.h.Meta.GitlabContent.PId
}

//GetProjectPath get gitlab project path
func (g *gitlabReader) GetProjectPath() string {
	return g.h.Meta.GitlabContent.Path
}

// ListAddonMeta relative path to repoURL/basePath
func (g *gitlabReader) ListAddonMeta() (addonCandidates map[string]SourceMeta, err error) {
	addonCandidates = make(map[string]SourceMeta)
	//the first dir is addonName
	path := g.GetProjectPath()
	tree, _, err := g.h.Client.Repositories.ListTree(g.GetProjectId(), &gitlab.ListTreeOptions{Path: &path})
	if err != nil {
		return nil, err
	}

	for _, node := range tree {
		if node.Type == TreeType {
			items, err := g.listAddonItem(make([]Item, 0), node.Path)
			if err != nil {
				return nil, err
			}
			addonCandidates[node.Name] = SourceMeta{
				Name:  node.Name,
				Items: items,
			}
		}
	}

	return addonCandidates, nil
}

func (g *gitlabReader) listAddonItem(item []Item, path string) ([]Item, error) {
	tree, _, err := g.h.Client.Repositories.ListTree(g.GetProjectId(), &gitlab.ListTreeOptions{Path: &path})
	if err != nil {
		return item, err
	}
	for _, node := range tree {
		switch node.Type {
		case TreeType:
			item, err = g.listAddonItem(item, node.Path)
			if err != nil {
				return nil, err
			}
		case BlobType:
			item = append(item, &GitLabItem{
				basePath: g.GetProjectPath(),
				tp:       FileType,
				path:     node.Path,
				name:     node.Name,
			})
		}
	}
	return item, nil
}

// ReadFile read file content from gitlab
func (g *gitlabReader) ReadFile(path string) (content string, err error) {
	ref := g.GetRef()
	getFile, _, err := g.h.Client.RepositoryFiles.GetFile(g.GetProjectId(), g.GetProjectPath()+"/"+path, &gitlab.GetFileOptions{Ref: &ref})
	if err != nil {
		return "", err
	}
	decodeString, err := base64.StdEncoding.DecodeString(getFile.Content)
	if err != nil {
		return "", err
	}
	return string(decodeString), nil
}

func (g *gitlabReader) RelativePath(item Item) string {
	return item.GetPath()
}
