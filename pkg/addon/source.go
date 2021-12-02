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
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/go-resty/resty/v2"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils"
)

const (
	// EOFError is error returned by xml parse
	EOFError string = "EOF"
	// DirType means a directory
	DirType = "dir"
	// FileType means a file
	FileType = "file"

	bucketTmpl        = "%s://%s.%s"
	singleOssFileTmpl = "%s/%s"
	listOssFileTmpl   = "%s?max-keys=1000&prefix=%s"
)

// Source is where to get addons
type Source interface {
	GetAddon(name string, opt ListOptions) (*types.Addon, error)
	ListAddons(opt ListOptions) ([]*types.Addon, error)
}

// GitAddonSource defines the information about the Git as addon source
type GitAddonSource struct {
	URL   string `json:"url,omitempty" validate:"required"`
	Path  string `json:"path,omitempty"`
	Token string `json:"token,omitempty"`
}

// OSSAddonSource is Addon source from alicloud OSS
type OSSAddonSource struct {
	EndPoint string `json:"end_point" validate:"required"`
	Bucket   string `json:"bucket"`
}

// GetAddon from OSSAddonSource
func (o *OSSAddonSource) GetAddon(name string, opt ListOptions) (*types.Addon, error) {
	reader, err := NewAsyncReader(o.EndPoint, o.Bucket, "", ossType)
	if err != nil {
		return nil, err
	}
	addon, err := GetSingleAddonFromReader(reader, name, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// ListAddons from OSSAddonSource
func (o *OSSAddonSource) ListAddons(opt ListOptions) ([]*types.Addon, error) {
	reader, err := NewAsyncReader(o.EndPoint, o.Bucket, "", ossType)
	if err != nil {
		return nil, err
	}
	addon, err := GetAddonsFromReader(reader, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// GetAddon get an addon info from GitAddonSource, can be used for get or enable
func (git *GitAddonSource) GetAddon(name string, opt ListOptions) (*types.Addon, error) {
	reader, err := NewAsyncReader(git.URL, git.Path, git.Token, gitType)
	if err != nil {
		return nil, err
	}
	addon, err := GetSingleAddonFromReader(reader, name, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// ListAddons list addons' info from GitAddonSource
func (git *GitAddonSource) ListAddons(opt ListOptions) ([]*types.Addon, error) {
	r, err := NewAsyncReader(git.URL, git.Path, git.Token, "git")
	if err != nil {
		return nil, err
	}
	gitAddons, err := GetAddonsFromReader(r, opt)
	if err != nil {
		return nil, err
	}
	return gitAddons, nil
}

// Item is a partial interface for github.RepositoryContent
type Item interface {
	// GetType return "dir" or "file"
	GetType() string
	GetPath() string
	GetName() string
}

// AsyncReader helps async read files of addon
type AsyncReader interface {
	// Read will return either file content or directory sub-paths
	// Read should accept relative path to github repo/path or OSS bucket
	Read(path string) (content string, subItem []Item, err error)
	// Addon returns a addon to be readed
	Addon() *types.Addon
	// SendErr to outside and quit
	SendErr(err error)
	// Mutex return an mutex for slice insert
	Mutex() *sync.Mutex
	// RelativePath return a relative path to GitHub repo/path or OSS bucket
	RelativePath(item Item) string
	// WithNewAddonAndMutex is mainly for copy the whole reader to read a new addon
	WithNewAddonAndMutex() AsyncReader
}

// baseReader will contain basic parts for async reading addon file
type baseReader struct {
	a       *types.Addon
	errChan chan error
	// mutex is needed when append to addon's Definitions/CUETemplate/YAMLTemplate slices
	mutex *sync.Mutex
}

// Addon for baseReader
func (b *baseReader) Addon() *types.Addon {
	return b.a
}

// SendErr to baseReader err channel
func (b *baseReader) SendErr(err error) {
	b.errChan <- err
}

// Mutex to lock baseReader addon's slice
func (b *baseReader) Mutex() *sync.Mutex {
	return b.mutex
}

// gitHelper helps get addon's file by git
type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

type gitReader struct {
	baseReader
	h *gitHelper
}

func (g *gitReader) WithNewAddonAndMutex() AsyncReader {
	return &gitReader{
		baseReader: baseReader{
			a:       &types.Addon{},
			errChan: make(chan error, 1),
			mutex:   &sync.Mutex{},
		},
		h: g.h,
	}
}

// Read relative path to repoURL/basePath
func (g *gitReader) Read(relativePath string) (content string, subItems []Item, err error) {
	var dirs []*github.RepositoryContent

	file, dirs, err := g.h.readRepo(relativePath)
	if err != nil {
		return
	}
	if file != nil {
		content, err = file.GetContent()
		return
	}
	for _, d := range dirs {
		subItems = append(subItems, d)
	}
	return
}

func (g *gitReader) RelativePath(item Item) string {
	absPath := strings.Split(item.GetPath(), "/")
	base := strings.Split(g.h.Meta.Path, "/")
	return path.Join(absPath[len(base):]...)
}

type ossReader struct {
	baseReader
	bucketEndPoint string
	client         *resty.Client
}

// OssItem is Item implement for OSS
type OssItem struct {
	tp   string
	path string
	name string
}

// GetType from OssItem
func (i OssItem) GetType() string {
	return i.tp
}

// GetPath from OssItem
func (i OssItem) GetPath() string {
	return i.path
}

// GetName from OssItem
func (i OssItem) GetName() string {
	return i.name
}

// Read from oss
func (o *ossReader) Read(readPath string) (content string, subItem []Item, err error) {
	if readPath == "." {
		readPath = ""
	}
	resp, err := o.client.R().Get(fmt.Sprintf(listOssFileTmpl, o.bucketEndPoint, readPath))

	if err != nil {
		return "", nil, errors.Wrapf(err, "read path %s fail", readPath)
	}
	list := ListBucketResult{}
	err = xml.Unmarshal(resp.Body(), &list)
	if err != nil && err.Error() != EOFError {
		return "", nil, err
	}
	var actualFiles []File
	for _, f := range list.Files {
		if f.Size > 0 {
			actualFiles = append(actualFiles, f)
		}
	}
	list.Files = actualFiles
	list.Count = len(actualFiles)
	if len(list.Files) == 1 && list.Files[0].Name == readPath {
		resp, err = o.client.R().Get(fmt.Sprintf(singleOssFileTmpl, o.bucketEndPoint, readPath))
		if err != nil {
			return "", nil, err
		}
		// This is a file
		return string(resp.Body()), nil, nil
	}
	// This is a path
	if err == nil {
		items := convert2OssItem(list.Files, readPath)
		return "", items, nil
	}

	return "", nil, errors.Wrap(err, "read oss fail")
}

func convert2OssItem(files []File, nowPath string) []Item {
	const slash = "/"
	var items []Item
	ps := strings.Split(path.Clean(nowPath), slash)
	pathExist := map[string]bool{}
	for _, f := range files {
		fPath := strings.Split(path.Clean(f.Name), slash)
		if ps[0] != "." {
			fPath = fPath[len(ps):]
		}
		name := fPath[0]
		if _, exist := pathExist[name]; exist {
			continue
		}
		pathExist[name] = true
		item := OssItem{
			path: f.Name,
			name: fPath[0],
			tp:   FileType,
		}
		if len(fPath) > 1 {
			item.path = path.Join(nowPath, item.name)
			item.tp = DirType
		}
		items = append(items, item)
	}
	return items
}

func (o *ossReader) RelativePath(item Item) string {
	return item.GetPath()
}

func (o *ossReader) WithNewAddonAndMutex() AsyncReader {
	return &ossReader{
		baseReader: baseReader{
			a:       &types.Addon{},
			errChan: make(chan error, 1),
			mutex:   &sync.Mutex{},
		},
		bucketEndPoint: o.bucketEndPoint,
		client:         o.client,
	}
}

// ReaderType marks where to read addon files
type ReaderType string

const (
	gitType ReaderType = "git"
	ossType ReaderType = "oss"
)

// NewAsyncReader create AsyncReader from
// 1. GitHub url and directory
// 2. OSS endpoint and bucket
func NewAsyncReader(baseURL, dirOrBucket, token string, rdType ReaderType) (AsyncReader, error) {
	bReader := baseReader{
		a:       &types.Addon{},
		errChan: make(chan error, 1),
		mutex:   &sync.Mutex{},
	}
	switch rdType {
	case gitType:
		baseURL = strings.TrimSuffix(baseURL, ".git")
		u, err := url.Parse(baseURL)
		if err != nil {
			return nil, errors.New("addon registry invalid")
		}
		u.Path = path.Join(u.Path, dirOrBucket)
		tp, content, err := utils.Parse(u.String())
		if err != nil || tp != utils.TypeGithub {
			return nil, err
		}
		gith := createGitHelper(content, token)
		return &gitReader{
			baseReader: bReader,
			h:          gith,
		}, nil
	case ossType:
		ossURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, err
		}
		var bucketEndPoint string
		if dirOrBucket == "" {
			bucketEndPoint = ossURL.String()
		} else {
			if ossURL.Scheme == "" {
				ossURL.Scheme = "https"
			}
			bucketEndPoint = fmt.Sprintf(bucketTmpl, ossURL.Scheme, dirOrBucket, ossURL.Host)
		}
		return &ossReader{
			baseReader:     bReader,
			bucketEndPoint: bucketEndPoint,
			client:         resty.New(),
		}, nil
	}
	return nil, errors.New("addon registry invalid")
}

// ListBucketResult describe a file list from OSS
type ListBucketResult struct {
	Files []File `xml:"Contents"`
	Count int    `xml:"KeyCount"`
}

type File struct {
	Name string `xml:"Key"`
	Size int    `xml:"Size"`
}
