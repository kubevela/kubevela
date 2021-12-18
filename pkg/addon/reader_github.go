package addon

import (
	"fmt"
	"path"
	"strings"

	"github.com/google/go-github/v32/github"

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
func (g *gitReader) ListAddonMeta() (subItems map[string]SourceMeta, err error) {
	_, dirs, err := g.h.readRepo("")
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
