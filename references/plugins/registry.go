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
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	"io"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"strings"

	"github.com/oam-dev/kubevela/apis/types"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"

	"github.com/oam-dev/kubevela/pkg/utils/common"
)

const DefaultRegistry = "default"

// Registry define a registry stores trait & component defs
type Registry interface {
	GetName() string
	GetURL() string
	GetCap(addonName string) (types.Capability, []byte, error)
	ListCaps() ([]types.Capability, error)
}

// GithubRegistry is Registry's implementation treat github url as resource
type GithubRegistry struct {
	URL          string `json:"url"`
	RegistryName string `json:"registry_name"`
	client       *github.Client
	cfg          *GithubContent
	ctx          context.Context
}

func NewRegistryFromConfig(config RegistryConfig) (Registry, error) {
	return NewRegistry(context.TODO(), config.Token, config.Name, config.URL)
}

// NewRegistry will create a registry implementation
func NewRegistry(ctx context.Context, token, registryName string, regURL string) (Registry, error) {
	tp, cfg, err := Parse(regURL)
	if err != nil {
		return nil, err
	}
	switch tp {
	case TypeGithub:
		var tc *http.Client
		if token != "" {
			ts := oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token},
			)
			tc = oauth2.NewClient(ctx, ts)
		}
		return GithubRegistry{
			URL:          cfg.URL,
			RegistryName: registryName,
			client:       github.NewClient(tc),
			cfg:          &cfg.GithubContent,
			ctx:          ctx,
		}, nil
	case TypeOss:
		var tc http.Client
		return OssRegistry{
			Client:       &tc,
			BucketURL:    fmt.Sprintf("https://%s/", cfg.BucketURL),
			RegistryName: registryName,
		}, nil
	case TypeLocal:
		_, err := os.Stat(cfg.AbsDir)
		if os.IsNotExist(err) {
			return LocalRegistry{}, err
		}
		return LocalRegistry{
			AbsPath:      cfg.AbsDir,
			RegistryName: registryName,
		}, nil
	case TypeUnknown:
		return nil, fmt.Errorf("not supported url")
	}

	return nil, fmt.Errorf("not supported url")
}

func ListRegistryConfig() ([]RegistryConfig, error) {

	defaultRegistryConfig := RegistryConfig{Name: DefaultRegistry, URL: "oss://registry.kubevela.net/"}
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(config))
	if err != nil {
		if os.IsNotExist(err) {
			err := StoreRepos([]RegistryConfig{defaultRegistryConfig})
			if err != nil {
				return nil, errors.Wrap(err, "error initialize default registry")
			}
			return ListRegistryConfig()
		}
		return nil, err
	}
	var regConfigs []RegistryConfig
	if err = yaml.Unmarshal(data, &regConfigs); err != nil {
		return nil, err
	}
	haveDefault := false
	for _, r := range regConfigs {
		if r.URL == defaultRegistryConfig.URL {
			haveDefault = true
			break
		}
	}
	if !haveDefault {
		regConfigs = append(regConfigs, defaultRegistryConfig)
	}
	return regConfigs, nil
}

func GetRegistry(regName string) (Registry, error) {
	regConfigs, err := ListRegistryConfig()
	if err != nil {
		return nil, err
	}
	for _, conf := range regConfigs {
		if conf.Name == regName {
			return NewRegistryFromConfig(conf)
		}
	}
	return nil, errors.Errorf("registry %s not found", regName)
}

func ListRegistry() ([]Registry, error) {
	regConfigs, err := ListRegistryConfig()

	if err != nil {
		return nil, err
	}

	regs := []Registry{}
	ctx := context.TODO()
	for _, conf := range regConfigs {
		reg, err := NewRegistry(ctx, conf.Token, conf.Name, conf.URL)
		if err != nil {
			fmt.Printf("error converting registry %s, URL is %s", conf.Name, conf.URL)
			continue
		}
		regs = append(regs, reg)
	}
	return regs, nil
}

func (g GithubRegistry) GetName() string {
	return g.RegistryName
}

func (g GithubRegistry) GetURL() string {
	return g.cfg.URL
}

// ListCaps list all capabilities of registry
func (g GithubRegistry) ListCaps() ([]types.Capability, error) {
	var addons []types.Capability

	itemContents, err := g.getRepoFile()
	if err != nil {
		return []types.Capability{}, err
	}
	for _, item := range itemContents {
		capa, err := item.toCapability()
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
	addon, err := repoFile.toCapability()
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
	*http.Client `json:"-"`
	BucketURL    string `json:"bucket_url"`
	RegistryName string `json:"registry_name"`
}

func (o OssRegistry) GetName() string {
	return o.RegistryName
}

func (o OssRegistry) GetURL() string {
	return o.BucketURL
}

// GetCap return capability object and raw data specified by cap name
func (o OssRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	filename := addonName + ".yaml"
	req, _ := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		o.BucketURL+filename,
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
	capa, err := rf.toCapability()
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
		capa, err := rf.toCapability()
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
		o.BucketURL+"?list-type=2",
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
			o.BucketURL+fileName,
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
	AbsPath      string `json:"abs_path"`
	RegistryName string `json:"registry_name"`
}

func (l LocalRegistry) GetName() string {
	return l.RegistryName
}

func (l LocalRegistry) GetURL() string {
	return l.AbsPath
}

// GetCap return capability object and raw data specified by cap name
func (l LocalRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	fileName := addonName + ".yaml"
	filePath := fmt.Sprintf("%s/%s", l.AbsPath, fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	file := RegistryFile{
		data: data,
		name: fileName,
	}
	capa, err := file.toCapability()
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	return capa, data, nil
}

// ListCaps list all capabilities of registry
func (l LocalRegistry) ListCaps() ([]types.Capability, error) {
	glob := filepath.Join(filepath.Clean(l.AbsPath), "*")
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
		}.toCapability()
		if err != nil {
			fmt.Printf("parsing file: %s err: %s\n", file, err)
			continue
		}
		capas = append(capas, capa)
	}
	return capas, nil
}

func (item RegistryFile) toCapability() (types.Capability, error) {
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

// Content contains different type of content needed when building Registry
type Content struct {
	OssContent
	GithubContent
	LocalContent
}

// LocalContent for local registry
type LocalContent struct {
	AbsDir string `json:"abs_dir"`
}

// OssContent for oss registry
type OssContent struct {
	BucketURL string `json:"bucket_url"`
}

// GithubContent for registry
type GithubContent struct {
	URL   string `json:"url"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref"`
}

// RegistryConfig is used to store registry config in file
type RegistryConfig struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token"`
}

// TypeLocal represents github
const TypeLocal = "local"

// TypeOss represent oss
const TypeOss = "oss"

// TypeGithub represents github
const TypeGithub = "github"

// TypeUnknown represents parse failed
const TypeUnknown = "unknown"

// Parse will parse config from address
func Parse(addr string) (string, *Content, error) {
	URL, err := url.Parse(addr)
	if err != nil {
		return "", nil, err
	}
	l := strings.Split(strings.TrimPrefix(URL.Path, "/"), "/")
	switch URL.Scheme {
	case "http", "https":
		switch URL.Host {
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
				return TypeGithub, &Content{
					GithubContent: GithubContent{
						URL:   addr,
						Owner: l[0],
						Repo:  l[1],
						Path:  strings.Join(l[4:], "/"),
						Ref:   l[3],
					},
				}, nil
			}
			// https://github.com/<owner>/<repo>/<path-to-dir>
			return TypeGithub, &Content{
					GithubContent: GithubContent{
						URL:   addr,
						Owner: l[0],
						Repo:  l[1],
						Path:  strings.Join(l[2:], "/"),
						Ref:   "", // use default branch
					},
				},
				nil
		case "api.github.com":
			if len(l) != 5 {
				return "", nil, errors.New("invalid format " + addr)
			}
			//https://api.github.com/repos/<owner>/<repo>/contents/<path-to-dir>
			return TypeGithub, &Content{
					GithubContent: GithubContent{
						URL:   addr,
						Owner: l[1],
						Repo:  l[2],
						Path:  l[4],
						Ref:   URL.Query().Get("ref"),
					},
				},
				nil
		default:
		}
	case "oss":
		return TypeOss, &Content{
			OssContent: OssContent{
				BucketURL: URL.Host,
			},
		}, nil
	case "file":
		return TypeLocal, &Content{
			LocalContent: LocalContent{
				AbsDir: URL.Path,
			},
		}, nil

	}

	return TypeUnknown, nil, nil
}

// StoreRepos will store registry repo locally
func StoreRepos(registries []RegistryConfig) error {
	config, err := system.GetRepoConfig()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(registries)
	if err != nil {
		return err
	}
	//nolint:gosec
	return os.WriteFile(config, data, 0644)
}

// ParseCapability will convert config from remote center to capability
func ParseCapability(mapper discoverymapper.DiscoveryMapper, data []byte) (types.Capability, error) {
	var obj = unstructured.Unstructured{Object: make(map[string]interface{})}
	err := yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		return types.Capability{}, err
	}
	switch obj.GetKind() {
	case "ComponentDefinition":
		var cd v1beta1.ComponentDefinition
		err = yaml.Unmarshal(data, &cd)
		if err != nil {
			return types.Capability{}, err
		}
		ref, err := util.ConvertWorkloadGVK2Definition(mapper, cd.Spec.Workload.Definition)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(cd.Name, ref.Name, cd.Annotations, cd.Labels, cd.Spec.Extension, types.TypeComponentDefinition, nil, cd.Spec.Schematic)
	case "TraitDefinition":
		var td v1beta1.TraitDefinition
		err = yaml.Unmarshal(data, &td)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(td.Name, td.Spec.Reference.Name, td.Annotations, td.Labels, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads, td.Spec.Schematic)
	case "ScopeDefinition":
		// TODO(wonderflow): support scope definition here.
	}
	return types.Capability{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}
