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

	"golang.org/x/oauth2"

	"github.com/google/go-github/v32/github"

	"github.com/cloud-native-application/rudrx/api/types"
	"github.com/crossplane/oam-kubernetes-runtime/apis/core/v1alpha2"

	"github.com/cloud-native-application/rudrx/pkg/utils/system"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type GithubContent struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Ref   string `json:"ref"`
}

//CapCenterConfig is used to store cap center config in file
type CapCenterConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Token   string `json:"token"`
}

type CenterClient interface {
	SyncCapabilityFromCenter() error
}

func NewCenterClient(ctx context.Context, name, address, token string) (CenterClient, error) {
	Type, cfg, err := Parse(address)
	if err != nil {
		return nil, err
	}
	switch Type {
	case TypeGithub:
		return NewGithubCenter(ctx, token, name, cfg)
	}
	return nil, errors.New("we only support github as repository now")
}

const TypeGithub = "github"
const TypeUnknown = "unknown"

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
		} else {
			// https://github.com/<owner>/<repo>/<path-to-dir>
			return TypeGithub, &GithubContent{
				Owner: l[0],
				Repo:  l[1],
				Path:  strings.Join(l[2:], "/"),
				Ref:   "", //use default branch
			}, nil
		}
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
		//TODO(wonderflow): support raw url and oss format in the future
	}
	return TypeUnknown, nil, nil
}

type RemoteCapability struct {
	// Name MUST be xxx.yaml
	Name string `json:"name"`
	Url  string `json:"download_url"`
	Sha  string `json:"sha"`
	// Type MUST be file
	Type string `json:"type"`
}

type RemoteCapabilities []RemoteCapability

//TODO(wonderflow): we can make default(built-in) repo configurable, then we should make default inside the answer
func LoadRepos() ([]CapCenterConfig, error) {
	config, err := system.GetRepoConfig()
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(config)
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

func StoreRepos(repos []CapCenterConfig) error {
	config, err := system.GetRepoConfig()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(repos)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(config, data, 0644)
}

func ParseAndSyncCapability(data []byte, syncDir string) (types.Capability, error) {
	var obj = unstructured.Unstructured{Object: make(map[string]interface{})}
	err := yaml.Unmarshal(data, &obj.Object)
	if err != nil {
		return types.Capability{}, err
	}
	switch obj.GetKind() {
	case "WorkloadDefinition":
		var rd v1alpha2.WorkloadDefinition
		err = yaml.Unmarshal(data, &rd)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(rd.Name, syncDir, rd.Spec.Reference.Name, rd.Spec.Extension, types.TypeWorkload, nil)
	case "TraitDefinition":
		var td v1alpha2.TraitDefinition
		err = yaml.Unmarshal(data, &td)
		if err != nil {
			return types.Capability{}, err
		}
		return HandleDefinition(td.Name, syncDir, td.Spec.Reference.Name, td.Spec.Extension, types.TypeTrait, td.Spec.AppliesToWorkloads)
	case "ScopeDefinition":
		//TODO(wonderflow): support scope definition here.
	}
	return types.Capability{}, fmt.Errorf("unknown definition Type %s", obj.GetKind())
}

type GithubCenter struct {
	client     *github.Client
	cfg        *GithubContent
	centerName string
	ctx        context.Context
}

var _ CenterClient = &GithubCenter{}

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

//TODO(wonderflow): currently we only sync by create, we also need to delete which not exist remotely.
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
	system.StatAndCreate(repoDir)
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
				return fmt.Errorf("decode github content %s err %v", *fileContent.Path, err)
			}
		}
		tmp, err := ParseAndSyncCapability(data, filepath.Join(dir, ".tmp"))
		if err != nil {
			fmt.Printf("parse definition of %s err %v\n", *fileContent.Name, err)
			continue
		}
		err = ioutil.WriteFile(filepath.Join(repoDir, tmp.CrdName+".yaml"), data, 0644)
		if err != nil {
			fmt.Printf("write definition %s to %s err %v\n", tmp.CrdName+".yaml", repoDir, err)
			continue
		}
		success++
	}
	fmt.Printf("successfully sync %d/%d from %s remote center\n", success, total, g.centerName)
	return nil
}
