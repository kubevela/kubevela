package plugins

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v32/github"
	"golang.org/x/oauth2"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// Registry define a registry stores trait & component defs
type Registry interface {
	GetCap(addonName string) (types.Capability, []byte, error)
	ListCaps() ([]types.Capability, error)
}

// GithubRegistry is Registry's implementation trait github url as resource
type GithubRegistry struct {
	client *github.Client
	cfg    *GithubContent
	ctx    context.Context
	name   string // to be used to cache registry
}

// NewRegistry will create a registry implementation
func NewRegistry(ctx context.Context, token, registryName string, regURL string) (Registry, error) {
	if strings.HasPrefix(regURL, "http") {
		// todo(qiaozp) support oss
		_, cfg, err := Parse(regURL)
		if err != nil {
			return nil, err
		}
		var tc *http.Client
		if token != "" {
			ts := oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token},
			)
			tc = oauth2.NewClient(ctx, ts)
		}
		return GithubRegistry{client: github.NewClient(tc), cfg: cfg, ctx: ctx, name: registryName}, nil
	} else if strings.HasPrefix(regURL, "file://") {
		dir := strings.TrimPrefix(regURL, "file://")
		_, err := os.Stat(dir)
		if os.IsNotExist(err) {
			return LocalRegistry{}, err
		}
		return LocalRegistry{absPath: dir}, nil
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
	repoFile := RepoFile{
		data: data,
		name: *fileContent.Name,
	}
	addon, err := repoFile.toAddon()
	if err != nil {
		return types.Capability{}, []byte{}, err
	}
	return addon, data, nil
}

func (g *GithubRegistry) getRepoFile() ([]RepoFile, error) {
	var items []RepoFile
	_, dirs, _, err := g.client.Repositories.GetContents(g.ctx, g.cfg.Owner, g.cfg.Repo, g.cfg.Path, &github.RepositoryContentGetOptions{Ref: g.cfg.Ref})
	if err != nil {
		return []RepoFile{}, err
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
		items = append(items, RepoFile{
			data: data,
			name: *fileContent.Name,
		})
	}
	return items, nil
}

// LocalRegistry is Registry's implementation trait local url as resource
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
	file := RepoFile{
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
		data, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		capa, err := RepoFile{
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
func (item RepoFile) toAddon() (types.Capability, error) {
	dm, err := (&common.Args{}).GetDiscoveryMapper()
	if err != nil {
		return types.Capability{}, err
	}
	capability, err := ParseAndSyncCapability(dm, item.data)
	if err != nil {
		return types.Capability{}, err
	}
	return capability, nil
}

// RepoFile contains a file item in github repo
type RepoFile struct {
	data []byte // file content
	name string // file's name
}
