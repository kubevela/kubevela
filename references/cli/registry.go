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

package cli

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/apis"
	"github.com/oam-dev/kubevela/references/docgen"
)

// NewRegistryCommand Manage Capability Center
func NewRegistryCommand(ioStream cmdutil.IOStreams, order string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage Registry",
		Long:  "Manage Registry of X-Definitions for extension.",
		Annotations: map[string]string{
			types.TagCommandOrder: order,
			types.TagCommandType:  types.TypeExtension,
		},
	}
	cmd.AddCommand(
		NewRegistryConfigCommand(ioStream),
		NewRegistryListCommand(ioStream),
		NewRegistryRemoveCommand(ioStream),
	)
	return cmd
}

// NewRegistryListCommand List all registry
func NewRegistryListCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "List all registry",
		Long:    "List all configured registry",
		Example: `vela registry ls`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listCapRegistrys(ioStreams)
		},
	}
	return cmd
}

// NewRegistryConfigCommand Configure (add if not exist) a registry, default is local (built-in capabilities)
func NewRegistryConfigCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config <registryName> <centerURL>",
		Short:   "Configure (add if not exist) a registry, default is local (built-in capabilities)",
		Long:    "Configure (add if not exist) a registry, default is local (built-in capabilities)",
		Example: `vela registry config my-registry https://github.com/oam-dev/catalog/tree/master/registry`,
		RunE: func(cmd *cobra.Command, args []string) error {
			argsLength := len(args)
			if argsLength < 2 {
				return errors.New("please set registry with <centerName> and <centerURL>")
			}
			capName := args[0]
			capURL := args[1]
			token := cmd.Flag("token").Value.String()
			if err := addRegistry(capName, capURL, token); err != nil {
				return err
			}
			ioStreams.Infof("Successfully configured registry %s\n", capName)
			return nil
		},
	}
	cmd.PersistentFlags().StringP("token", "t", "", "Github Repo token")
	return cmd
}

// NewRegistryRemoveCommand Remove specified registry
func NewRegistryRemoveCommand(ioStreams cmdutil.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Aliases: []string{"rm"},
		Use:     "remove <centerName>",
		Short:   "Remove specified registry",
		Long:    "Remove specified registry",
		Example: "vela registry remove mycenter",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("you must specify <name> for capability center you want to remove")
			}
			centerName := args[0]
			msg, err := removeRegistry(centerName)
			if err == nil {
				ioStreams.Info(msg)
			}
			return err
		},
	}
	return cmd
}

func listCapRegistrys(ioStreams cmdutil.IOStreams) error {
	table := newUITable()
	table.MaxColWidth = 80
	table.AddRow("NAME", "URL")

	registrys, err := ListRegistryConfig()
	if err != nil {
		return errors.Wrap(err, "list registry error")
	}
	for _, c := range registrys {
		tokenShow := ""
		if len(c.Token) > 0 {
			tokenShow = "***"
		}
		table.AddRow(c.Name, c.URL, tokenShow)
	}
	ioStreams.Info(table.String())
	return nil
}

// addRegistry will add a registry
func addRegistry(regName, regURL, regToken string) error {
	regConfig := apis.RegistryConfig{
		Name: regName, URL: regURL, Token: regToken,
	}
	repos, err := ListRegistryConfig()
	if err != nil {
		return err
	}
	var updated bool
	for idx, r := range repos {
		if r.Name == regConfig.Name {
			repos[idx] = regConfig
			updated = true
			break
		}
	}
	if !updated {
		repos = append(repos, regConfig)
	}
	if err = StoreRepos(repos); err != nil {
		return err
	}
	return nil
}

// removeRegistry will remove a registry from local
func removeRegistry(regName string) (string, error) {
	var message string
	var err error

	regConfigs, err := ListRegistryConfig()
	if err != nil {
		return message, err
	}
	found := false
	for idx, r := range regConfigs {
		if r.Name == regName {
			regConfigs = append(regConfigs[:idx], regConfigs[idx+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("registry %s not found", regName), nil
	}
	if err = StoreRepos(regConfigs); err != nil {
		return message, err
	}
	message = fmt.Sprintf("Successfully remove registry %s", regName)
	return message, err
}

// DefaultRegistry is default registry
const DefaultRegistry = "default"

// Registry define a registry used to get and list types.Capability
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

// NewRegistryFromConfig return Registry interface to get capabilities
func NewRegistryFromConfig(config apis.RegistryConfig) (Registry, error) {
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

// ListRegistryConfig will get all registry config stored in local
// this will return at least one config, which is DefaultRegistry
func ListRegistryConfig() ([]apis.RegistryConfig, error) {

	defaultRegistryConfig := apis.RegistryConfig{Name: DefaultRegistry, URL: "oss://registry.kubevela.net/"}
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(config))
	if err != nil {
		if os.IsNotExist(err) {
			err := StoreRepos([]apis.RegistryConfig{defaultRegistryConfig})
			if err != nil {
				return nil, errors.Wrap(err, "error initialize default registry")
			}
			return ListRegistryConfig()
		}
		return nil, err
	}
	var regConfigs []apis.RegistryConfig
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

// GetRegistry get a Registry implementation by name
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

// GetName will return registry name
func (g GithubRegistry) GetName() string {
	return g.RegistryName
}

// GetURL will return github registry url
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
	capa, err := repoFile.toCapability()
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	capa.Source = &types.Source{RepoName: g.RegistryName}
	return capa, data, nil
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

// GetName return name of OssRegistry
func (o OssRegistry) GetName() string {
	return o.RegistryName
}

// GetURL return URL of OssRegistry's bucket
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
	capa.Source = &types.Source{RepoName: o.RegistryName}

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

// GetName return name of LocalRegistry
func (l LocalRegistry) GetName() string {
	return l.RegistryName
}

// GetURL return path of LocalRegistry
func (l LocalRegistry) GetURL() string {
	return l.AbsPath
}

// GetCap return capability object and raw data specified by cap name
func (l LocalRegistry) GetCap(addonName string) (types.Capability, []byte, error) {
	fileName := addonName + ".yaml"
	filePath := fmt.Sprintf("%s/%s", l.AbsPath, fileName)
	data, err := os.ReadFile(filePath) // nolint
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
	capa.Source = &types.Source{RepoName: l.RegistryName}

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
func StoreRepos(registries []apis.RegistryConfig) error {
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
	return docgen.ParseCapabilityFromUnstructured(mapper, nil, obj)
}
