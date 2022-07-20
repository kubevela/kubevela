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
	"net/url"
	"path"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"github.com/xanzy/go-gitlab"

	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// EOFError is error returned by xml parse
	EOFError string = "EOF"
	// DirType means a directory
	DirType = "dir"
	// FileType means a file
	FileType = "file"
	// BlobType means a blob
	BlobType = "blob"
	// TreeType means a tree
	TreeType = "tree"

	bucketTmpl        = "%s://%s.%s"
	singleOSSFileTmpl = "%s/%s"
	listOSSFileTmpl   = "%s?max-keys=1000&prefix=%s"
)

// Source is where to get addons, Registry implement this interface
type Source interface {
	GetUIData(meta *SourceMeta, opt ListOptions) (*UIData, error)
	ListUIData(registryAddonMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error)
	GetInstallPackage(meta *SourceMeta, uiData *UIData) (*InstallPackage, error)
	ListAddonMeta() (map[string]SourceMeta, error)
}

// GitAddonSource defines the information about the Git as addon source
type GitAddonSource struct {
	URL   string `json:"url,omitempty" validate:"required"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// GiteeAddonSource defines the information about the Gitee as addon source
type GiteeAddonSource struct {
	URL   string `json:"url,omitempty" validate:"required"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// GitlabAddonSource defines the information about the Gitlab as addon source
type GitlabAddonSource struct {
	URL   string `json:"url,omitempty" validate:"required"`
	Repo  string `json:"repo,omitempty" validate:"required"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// HelmSource  defines the information about the helm repo addon source
type HelmSource struct {
	URL             string `json:"url,omitempty" validate:"required"`
	InsecureSkipTLS bool   `json:"insecureSkipTLS,omitempty"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
}

// SafeCopier is an interface to copy Struct without sensitive fields, such as Token, Username, Password
type SafeCopier interface {
	SafeCopy() interface{}
}

// SafeCopy hides field Token
func (g *GitAddonSource) SafeCopy() *GitAddonSource {
	if g == nil {
		return nil
	}
	return &GitAddonSource{
		URL:  g.URL,
		Path: g.Path,
	}
}

// SafeCopy hides field Token
func (g *GiteeAddonSource) SafeCopy() *GiteeAddonSource {
	if g == nil {
		return nil
	}
	return &GiteeAddonSource{
		URL:  g.URL,
		Path: g.Path,
	}
}

// SafeCopy hides field Token
func (g *GitlabAddonSource) SafeCopy() *GitlabAddonSource {
	if g == nil {
		return nil
	}
	return &GitlabAddonSource{
		URL:  g.URL,
		Repo: g.Repo,
		Path: g.Path,
	}
}

// SafeCopy hides field Username, Password
func (h *HelmSource) SafeCopy() *HelmSource {
	if h == nil {
		return nil
	}
	return &HelmSource{
		URL: h.URL,
	}
}

// Item is a partial interface for github.RepositoryContent
type Item interface {
	// GetType return "dir" or "file"
	GetType() string
	GetPath() string
	GetName() string
}

// SourceMeta record the whole metadata of an addon
type SourceMeta struct {
	Name  string
	Items []Item
}

// ClassifyItemByPattern will filter and classify addon data, data will be classified by pattern it meets
func ClassifyItemByPattern(meta *SourceMeta, r AsyncReader) map[string][]Item {
	var p = make(map[string][]Item)
	for _, it := range meta.Items {
		pt := GetPatternFromItem(it, r, meta.Name)
		if pt == "" {
			continue
		}
		items := p[pt]
		items = append(items, it)
		p[pt] = items
	}
	return p
}

// AsyncReader helps async read files of addon
type AsyncReader interface {
	// ListAddonMeta will return directory tree contain addon metadata only
	ListAddonMeta() (addonCandidates map[string]SourceMeta, err error)

	// ReadFile should accept relative path to github repo/path or OSS bucket, and report the file content
	ReadFile(path string) (content string, err error)

	// RelativePath return a relative path to GitHub repo/path or OSS bucket/path
	RelativePath(item Item) string
}

// pathWithParent joins path with its parent directory, suffix slash is reserved
func pathWithParent(subPath, parent string) string {
	actualPath := path.Join(parent, subPath)
	if strings.HasSuffix(subPath, "/") {
		actualPath += "/"
	}
	return actualPath
}

// ReaderType marks where to read addon files
type ReaderType string

const (
	gitType    ReaderType = "git"
	ossType    ReaderType = "oss"
	giteeType  ReaderType = "gitee"
	gitlabType ReaderType = "gitlab"
)

// NewAsyncReader create AsyncReader from
// 1. GitHub url and directory
// 2. OSS endpoint and bucket
func NewAsyncReader(baseURL, bucket, repo, subPath, token string, rdType ReaderType) (AsyncReader, error) {

	switch rdType {
	case gitType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		gith := createGitHelper(content, token)
		return &gitReader{
			h: gith,
		}, nil
	case ossType:
		ossURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, err
		}
		var bucketEndPoint string
		if bucket == "" {
			bucketEndPoint = ossURL.String()
		} else {
			if ossURL.Scheme == "" {
				ossURL.Scheme = "https"
			}
			bucketEndPoint = fmt.Sprintf(bucketTmpl, ossURL.Scheme, bucket, ossURL.Host)
		}
		return &ossReader{
			bucketEndPoint: bucketEndPoint,
			path:           subPath,
			client:         resty.New(),
		}, nil
	case giteeType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		u.Path = path.Join(u.Path, subPath)
		_, content, err := utils.Parse(u.String())
		if err != nil {
			return nil, err
		}
		gitee := createGiteeHelper(content, token)
		return &giteeReader{
			h: gitee,
		}, nil
	case gitlabType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		_, content, err := utils.ParseGitlab(u.String(), repo)
		content.GitlabContent.Path = subPath
		if err != nil {
			return nil, err
		}
		gitlabHelper, err := createGitlabHelper(content, token)
		if err != nil {
			return nil, errors.New("addon registry connect fail")
		}

		err = gitlabHelper.getGitlabProject(content)
		if err != nil {
			return nil, err
		}

		return &gitlabReader{
			h: gitlabHelper,
		}, nil
	}
	return nil, fmt.Errorf("invalid addon registry type '%s'", rdType)
}

// getGitlabProject get gitlab project , set project id
func (h *gitlabHelper) getGitlabProject(content *utils.Content) error {
	projectURL := content.GitlabContent.Owner + "/" + content.GitlabContent.Repo
	projects, _, err := h.Client.Projects.GetProject(projectURL, &gitlab.GetProjectOptions{})
	if err != nil {
		return err
	}
	content.GitlabContent.PId = projects.ID

	return nil
}

// BuildReader will build a AsyncReader from registry, AsyncReader are needed to read addon files
func (r *Registry) BuildReader() (AsyncReader, error) {
	if r.OSS != nil {
		o := r.OSS
		return NewAsyncReader(o.Endpoint, o.Bucket, "", o.Path, "", ossType)
	}
	if r.Git != nil {
		g := r.Git
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, gitType)
	}
	if r.Gitee != nil {
		g := r.Gitee
		return NewAsyncReader(g.URL, "", "", g.Path, g.Token, giteeType)
	}
	if r.Gitlab != nil {
		g := r.Gitlab
		return NewAsyncReader(g.URL, "", g.Repo, g.Path, g.Token, gitlabType)
	}
	return nil, errors.New("registry don't have enough info to build a reader")
}

// GetUIData get UIData of an addon
func (r *Registry) GetUIData(meta *SourceMeta, opt ListOptions) (*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	addon, err := GetUIDataFromReader(reader, meta, opt)
	if err != nil {
		return nil, err
	}
	if len(addon.GlobalParameters) != 0 {
		addon.Parameters = addon.GlobalParameters
	}
	return addon, nil
}

// ListUIData list UI data from addon registry
func (r *Registry) ListUIData(registryAddonMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return ListAddonUIDataFromReader(reader, registryAddonMeta, r.Name, opt)
}

// GetInstallPackage get install package which is all needed to enable an addon from addon registry
func (r *Registry) GetInstallPackage(meta *SourceMeta, uiData *UIData) (*InstallPackage, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return GetInstallPackageFromReader(reader, meta, uiData)
}

// ListAddonMeta list addon file meta(path and name) from a registry
func (r *Registry) ListAddonMeta() (map[string]SourceMeta, error) {
	reader, err := r.BuildReader()
	if err != nil {
		return nil, err
	}
	return reader.ListAddonMeta()
}
