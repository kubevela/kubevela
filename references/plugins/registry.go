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
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/oam-dev/kubevela/pkg/utils"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Registry define a registry stores trait & component defs
type Registry interface {
	GetCap(addonName string) (types.Capability, []byte, error)
	ListCaps() ([]types.Capability, error)
}

// GithubRegistry is Registry's implementation treat github url as resource
type GithubRegistry struct {
	client     *github.Client
	cfg        *utils.GithubContent
	ctx        context.Context
	centerName string // to be used to cache registry
}

// NewRegistry will create a registry implementation
func NewRegistry(ctx context.Context, token, registryName string, regURL string) (Registry, error) {
	tp, cfg, err := utils.Parse(regURL)
	if err != nil {
		return nil, err
	}
	switch tp {
	case utils.TypeGithub:
		var tc *http.Client
		if token != "" {
			ts := oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token},
			)
			tc = oauth2.NewClient(ctx, ts)
		}
		return GithubRegistry{client: github.NewClient(tc), cfg: &cfg.GithubContent, ctx: ctx, centerName: registryName}, nil
	case utils.TypeOss:
		var tc http.Client
		return OssRegistry{
			Client:    &tc,
			bucketURL: fmt.Sprintf("https://%s/", cfg.BucketURL),
		}, nil
	case utils.TypeLocal:
		_, err := os.Stat(cfg.AbsDir)
		if os.IsNotExist(err) {
			return LocalRegistry{}, err
		}
		return LocalRegistry{absPath: cfg.AbsDir}, nil
	case utils.TypeUnknown:
		return nil, fmt.Errorf("not supported url")
	}

	return nil, fmt.Errorf("not supported url")
}

// ListCaps list all capabilities of registry
func (g GithubRegistry) ListCaps() ([]types.Capability, error) {
	var addons []types.Capability

	itemContents, err := g.getRepoFile()
	if err != nil {
		return []types.Capability{}, err
	}
	for _, item := range itemContents {
		capa, err := item.toAddon()
		if err != nil {
			fmt.Printf("parse definition of %s err %v\n", item.name, err)
			continue
		}
		addons = append(addons, capa)
	}
	return addons, nil
}

// GetCap return capability object and raw data specified by cap name
func (g GithubRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	fileContent, _, _, err := g.client.Repositories.GetContents(context.Background(), g.cfg.Owner, g.cfg.Repo, fmt.Sprintf("%s/%s.yaml", g.cfg.Path, addonName), &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	var data []byte
	if *fileContent.Encoding == "base64" {
		data, err = base64.StdEncoding.DecodeString(*fileContent.Content)
		if err != nil {
			fmt.Printf("decode github content %s err %s\n", fileContent.GetPath(), err)
		}
	}
	repoFile := RegistryFile{
		data: data,
		name: *fileContent.Name,
	}
	addon, err := repoFile.toAddon()
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	return addon, data, nil
}

func (g *GithubRegistry) getRepoFile() ([]RegistryFile, error) {
	var items []RegistryFile
	_, dirs, _, err := g.client.Repositories.GetContents(g.ctx, g.cfg.Owner, g.cfg.Repo, g.cfg.Path, &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
	if err != nil {
		return []RegistryFile{}, err
	}
	for _, repoItem := range dirs {
		if *repoItem.Type != "file" {
			continue
		}
		fileContent, _, _, err := g.client.Repositories.GetContents(g.ctx, g.cfg.Owner, g.cfg.Repo, *repoItem.Path, &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
		if err != nil {
			fmt.Printf("Getting content URL %s error: %s\n", repoItem.GetURL(), err)
			continue
		}
		var data []byte
		if *fileContent.Encoding == "base64" {
			data, err = base64.StdEncoding.DecodeString(*fileContent.Content)
			if err != nil {
				fmt.Printf("decode github content %s err %s\n", fileContent.GetPath(), err)
				continue
			}
		}
		items = append(items, RegistryFile{
			data: data,
			name: *fileContent.Name,
		})
	}
	return items, nil
}

// OssRegistry is Registry's implementation treat OSS url as resource
type OssRegistry struct {
	*http.Client
	bucketURL  string
	centerName string
}

// GetCap return capability object and raw data specified by cap name
func (o OssRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	filename := addonName + ".yaml"
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		o.bucketURL+filename,
		nil,
	)
	resp, err := o.Client.Do(req)
	if err != nil {
		return types.Capability{}, nil, err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return types.Capability{}, nil, err
	}
	rf := RegistryFile{
		data: data,
		name: filename,
	}
	capa, err := rf.toAddon()
	if err != nil {
		return types.Capability{}, nil, err
	}

	return capa, data, nil
}

// ListCaps list all capabilities of registry
func (o OssRegistry) ListCaps() ([]types.Capability, error) {
	rfs, err := o.getRegFiles()
	if err != nil {
		return []types.Capability{}, errors.Wrap(err, "Get raw files fail")
	}
	capas := make([]types.Capability, 0)

	for _, rf := range rfs {
		capa, err := rf.toAddon()
		if err != nil {
			fmt.Printf("[WARN] Parse file %s fail: %s\n", rf.name, err.Error())
		}
		capas = append(capas, capa)
	}
	return capas, nil
}
func (o OssRegistry) getRegFiles() ([]RegistryFile, error) {
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		o.bucketURL+"?list-type=2",
		nil,
	)
	resp, err := o.Client.Do(req)
	if err != nil {
		return []RegistryFile{}, err
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return []RegistryFile{}, err
	}
	list := &ListBucketResult{}
	err = xml.Unmarshal(data, list)
	if err != nil {
		return []RegistryFile{}, err
	}
	rfs := make([]RegistryFile, 0)

	for _, fileName := range list.File {
		req, _ := http.NewRequestWithContext(
			context.Background(),
			http.MethodGet,
			o.bucketURL+fileName,
			nil,
		)
		resp, err := o.Client.Do(req)
		if err != nil {
			fmt.Printf("[WARN] %s download fail\n", fileName)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		rf := RegistryFile{
			data: data,
			name: fileName,
		}
		rfs = append(rfs, rf)

	}
	return rfs, nil
}

// LocalRegistry is Registry's implementation treat local url as resource
type LocalRegistry struct {
	absPath string
}

// GetCap return capability object and raw data specified by cap name
func (l LocalRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	fileName := addonName + ".yaml"
	filePath := fmt.Sprintf("%s/%s", l.absPath, fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	file := RegistryFile{
		data: data,
		name: fileName,
	}
	capa, err := file.toAddon()
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	return capa, data, nil
}

// ListCaps list all capabilities of registry
func (l LocalRegistry) ListCaps() ([]types.Capability, error) {
	glob := filepath.Join(filepath.Clean(l.absPath), "*")
	files, _ := filepath.Glob(glob)
	capas := make([]types.Capability, 0)
	for _, file := range files {
		// nolint:gosec
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		capa, err := RegistryFile{
			data: data,
			name: path.Base(file),
		}.toAddon()
		if err != nil {
			fmt.Printf("parsing file: %s err: %s\n", file, err)
			continue
		}
		capas = append(capas, capa)
	}
	return capas, nil
}
func (item RegistryFile) toAddon() (types.Capability, error) {
	dm, err := (&common.Args{}).GetDiscoveryMapper()
	if err != nil {
		return types.Capability{}, err
	}
	capability, err := ParseCapability(dm, item.data)
	if err != nil {
		return types.Capability{}, err
	}
	return capability, nil
}

// RegistryFile describes a file item in registry
type RegistryFile struct {
	data []byte // file content
	name string // file's name
}

// ListBucketResult describe a file list from OSS
type ListBucketResult struct {
	File  []string `xml:"Contents>Key"`
	Count int      `xml:"KeyCount"`
}
