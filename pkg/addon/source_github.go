package addon

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/v32/github"

	"github.com/oam-dev/kubevela/pkg/utils"
)

// gitHelper helps get addon's file by git
type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

type gitReader struct {
	h *gitHelper
}

// ListAddonMeta relative path to repoURL/basePath
func (g *gitReader) ListAddonMeta(relativePath string) (subItems map[string]SourceMeta, err error) {
	_, dirs, err := g.h.readRepo(relativePath)
	if err != nil {
		return
	}
	var items []Item
	for _, d := range dirs {
		items = append(items, d)
	}
	return map[string]SourceMeta{"TODO": {Name: "TODO", Items: items}}, nil
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
	base := strings.Split(g.h.Meta.Path, "/")
	return path.Join(absPath[len(base):]...)
}

// GetUIMeta get an addon info from GitAddonSource, can be used for get or enable
func (git *GitAddonSource) GetUIMeta(meta *SourceMeta, opt ListOptions) (*UIData, error) {
	reader, err := NewAsyncReader(git.URL, "", git.Path, git.Token, gitType)
	if err != nil {
		return nil, err
	}
	addon, err := GetUIMetaFromReader(reader, meta, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// ListRegistryMeta will list registry add meta for cache
func (git *GitAddonSource) ListRegistryMeta() (map[string]SourceMeta, error) {
	r, err := NewAsyncReader(git.URL, "", git.Path, git.Token, gitType)
	if err != nil {
		return nil, err
	}
	return r.ListAddonMeta(".")
}

// ListUIData list addons' info from GitAddonSource
func (git *GitAddonSource) ListUIData(registryMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error) {
	r, err := NewAsyncReader(git.URL, "", git.Path, git.Token, gitType)
	if err != nil {
		return nil, err
	}
	gitAddons, err := GetAddonUIMetaFromReader(r, registryMeta, opt)
	if err != nil {
		return nil, err
	}
	return gitAddons, nil
}

// GetInstallPackage get install package for addon
func (git *GitAddonSource) GetInstallPackage(meta *SourceMeta, uiMeta *UIData) (*InstallPackage, error) {
	r, err := NewAsyncReader(git.URL, "", git.Path, git.Token, gitType)
	if err != nil {
		return nil, err
	}
	return GetInstallPackageFromReader(r, meta, uiMeta)
}
