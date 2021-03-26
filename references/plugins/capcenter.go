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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
)

// GithubContent for cap center
type GithubContent struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref"`
}

// CapCenterConfig is used to store cap center config in file
type CapCenterConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Token   string `json:"token"`
}

// CenterClient defines an interface for cap center client
type CenterClient interface {
	SyncCapabilityFromCenter() error
}

// NewCenterClient create a client from type
func NewCenterClient(ctx context.Context, name, address, token string) (CenterClient, error) {
	Type, cfg, err := Parse(address)
	if err != nil {
		return nil, err
	}
	switch Type {
	case TypeGithub:
		return NewGithubCenter(ctx, token, name, cfg)
	default:
	}
	return nil, errors.New("we only support github as repository now")
}

// TypeGithub represents github
const TypeGithub = "github"

// TypeUnknown represents parse failed
const TypeUnknown = "unknown"

// Parse will parse config from address
func Parse(addr string) (string, *GithubContent, error) {
	url, err := url.Parse(addr)
	if err != nil {
		return "", nil, err
	}
	l := strings.Split(strings.TrimPrefix(url.Path, "/"), "/")
	switch url.Host {
	case "github.com":
		// We support two valid format:
		// 1. https://github.com/<owner>/<repo>/tree/<branch>/<path-to-dir>
		// 2. https://github.com/<owner>/<repo>/<path-to-dir>
		if len(l) < 3 {
			return "", nil, errors.New("invalid format " + addr)
		}
		if l[2] == "tree" {
			// https://github.com/<owner>/<repo>/tree/<branch>/<path-to-dir>
			if len(l) < 5 {
				return "", nil, errors.New("invalid format " + addr)
			}
			return TypeGithub, &GithubContent{
				Owner: l[0],
				Repo:  l[1],
				Path:  strings.Join(l[4:], "/"),
				Ref:   l[3],
			}, nil
		}
		// https://github.com/<owner>/<repo>/<path-to-dir>
		return TypeGithub, &GithubContent{
			Owner: l[0],
			Repo:  l[1],
			Path:  strings.Join(l[2:], "/"),
			Ref:   "", // use default branch
		}, nil
	case "api.github.com":
		if len(l) != 5 {
			return "", nil, errors.New("invalid format " + addr)
		}
		//https://api.github.com/repos/<owner>/<repo>/contents/<path-to-dir>
		return TypeGithub, &GithubContent{
			Owner: l[1],
			Repo:  l[2],
			Path:  l[4],
			Ref:   url.Query().Get("ref"),
		}, nil
	default:
		// TODO(wonderflow): support raw url and oss format in the future
	}
	return TypeUnknown, nil, nil
}

// RemoteCapability defines the capability discovered from remote cap center
type RemoteCapability struct {
	// Name MUST be xxx.yaml
	Name string `json:"name"`
	URL  string `json:"downloadUrl"`
	Sha  string `json:"sha"`
	// Type MUST be file
	Type string `json:"type"`
}

// RemoteCapabilities is slice of cap center
type RemoteCapabilities []RemoteCapability

// LoadRepos will load all cap center repos
// TODO(wonderflow): we can make default(built-in) repo configurable, then we should make default inside the answer
func LoadRepos() ([]CapCenterConfig, error) {
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(filepath.Clean(config))
	if err != nil {
		if os.IsNotExist(err) {
			return []CapCenterConfig{}, nil
		}
		return nil, err
	}
	var repos []CapCenterConfig
	if err = yaml.Unmarshal(data, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

// StoreRepos will store cap center repo locally
func StoreRepos(repos []CapCenterConfig) error {
	config, err := system.GetRepoConfig()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(repos)
	if err != nil {
		return err
	}
	//nolint:gosec
	return ioutil.WriteFile(config, data, 0644)
}

// ParseAndSyncCapability will convert config from remote center to capability
func ParseAndSyncCapability(mapper discoverymapper.DiscoveryMapper, data []byte) (types.Capability, error) {
	var obj = unstructured.Unstructured{Object: make(map[string]interface{})}
	err := yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		return types.Capability{}, err
	}
	switch obj.GetKind() {
	case "ComponentDefinition":
		var rd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(data, &rd)
		if err != nil {
			return types.Capability{}, err
		}
		ref, err := util.ConvertWorkloadGVK2Definition(mapper, rd.Spec.Workload.Definition)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(rd.Name, ref.Name, rd.Annotations, rd.Spec.Extension, types.TypeComponentDefinition, nil, rd.Spec.Schematic)
	case "TraitDefinition":
		var td v1beta1.TraitDefinition
		err = yaml.Unmarshal(data, &td)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(td.Name, td.Spec.Reference.Name, td.Annotations, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads, td.Spec.Schematic)
	case "ScopeDefinition":
		// TODO(wonderflow): support scope definition here.
	}
	return types.Capability{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}

// GithubCenter implementation of cap center
type GithubCenter struct {
	client     *github.Client
	cfg        *GithubContent
	centerName string
	ctx        context.Context
}

var _ CenterClient = &GithubCenter{}

// NewGithubCenter will create client by github center implementation
func NewGithubCenter(ctx context.Context, token, centerName string, r *GithubContent) (*GithubCenter, error) {
	var tc *http.Client
	if token != "" {
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: token},
		)
		tc = oauth2.NewClient(ctx, ts)
	}
	return &GithubCenter{client: github.NewClient(tc), cfg: r, centerName: centerName, ctx: ctx}, nil
}

// SyncCapabilityFromCenter will sync capability from github cap center
// TODO(wonderflow): currently we only sync by create, we also need to delete which not exist remotely.
func (g *GithubCenter) SyncCapabilityFromCenter() error {
	_, dirs, _, err := g.client.Repositories.GetContents(g.ctx, g.cfg.Owner, g.cfg.Repo, g.cfg.Path, &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
	if err != nil {
		return err
	}
	dir, err := system.GetCapCenterDir()
	if err != nil {
		return err
	}
	repoDir := filepath.Join(dir, g.centerName)
	_, _ = system.CreateIfNotExist(repoDir)
	c := &common.Args{}
	dm, err := c.GetDiscoveryMapper()
	if err != nil {
		return err
	}
	var success, total int
	for _, addon := range dirs {
		if *addon.Type != "file" {
			continue
		}
		total++
		fileContent, _, _, err := g.client.Repositories.GetContents(g.ctx, g.cfg.Owner, g.cfg.Repo, *addon.Path, &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
		if err != nil {
			return err
		}
		var data = []byte(*fileContent.Content)
		if *fileContent.Encoding == "base64" {
			data, err = base64.StdEncoding.DecodeString(*fileContent.Content)
			if err != nil {
				return fmt.Errorf("decode github content %s err %w", *fileContent.Path, err)
			}
		}
		tmp, err := ParseAndSyncCapability(dm, data)
		if err != nil {
			fmt.Printf("parse definition of %s err %v\n", *fileContent.Name, err)
			continue
		}
		//nolint:gosec
		err = ioutil.WriteFile(filepath.Join(repoDir, tmp.Name+".yaml"), data, 0644)
		if err != nil {
			fmt.Printf("write definition %s to %s err %v\n", tmp.Name+".yaml", repoDir, err)
			continue
		}
		success++
	}
	fmt.Printf("successfully sync %d/%d from %s remote center\n", success, total, g.centerName)
	return nil
}
