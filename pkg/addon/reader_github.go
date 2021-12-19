package addon

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"

	"github.com/oam-dev/kubevela/pkg/utils"
)

var _ AsyncReader = &gitReader{}

// gitHelper helps get addon's file by git
type gitHelper struct {
	Client *github.Client
	Meta   *utils.Content
}

type gitReader struct {
	h *gitHelper
}

// ListAddonMeta relative path to repoURL/basePath
func (g *gitReader) ListAddonMeta() (map[string]SourceMeta, error) {
	subItems := make(map[string]SourceMeta)
	_, items, err := g.h.readRepo("")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		// single addon
		addonName := path.Base(item.GetPath())
		addonMeta, err := g.listAddonMeta(g.RelativePath(item))
		if err != nil {
			return nil, errors.Wrapf(err, "fail to get addon meta of %s", addonName)
		}
		subItems[addonName] = SourceMeta{Name: addonName, Items: addonMeta}
	}
	return subItems, nil
}

func (g *gitReader) listAddonMeta(dirPath string) ([]Item, error) {
	_, items, err := g.h.readRepo(dirPath)
	if err != nil {
		return nil, err
	}
	res := make([]Item, 0)
	for _, item := range items {
		switch item.GetType() {
		case FileType:
			res = append(res, item)
		case DirType:
			subItems, err := g.listAddonMeta(g.RelativePath(item))
			if err != nil {
				return nil, err
			}
			for _, i := range subItems {
				res = append(res, i)
			}
		}
	}
	return res, nil
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
