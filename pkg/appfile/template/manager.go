package template

import (
	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/plugins"
)

type Manager interface {
	IsTrait(key string) bool
	LoadTemplate(key string) (defName, tmpl string)
}

func Load() (Manager, error) {
	caps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return nil, err
	}
	m := newManager()
	for _, cap := range caps {
		t := &Template{}
		t.Captype = cap.Type
		t.Raw = cap.CueTemplate
		t.DefName = cap.DefName
		m.Templates[cap.Name] = t
	}
	return m, nil
}

type Template struct {
	Captype types.CapType
	Raw     string
	DefName string
}

type manager struct {
	Templates map[string]*Template
}

func newManager() *manager {
	return &manager{
		Templates: make(map[string]*Template),
	}
}

func (m *manager) IsTrait(key string) bool {
	t, ok := m.Templates[key]
	if !ok {
		return false
	}
	return t.Captype == types.TypeTrait
}

func (m *manager) LoadTemplate(key string) (string, string) {
	t, ok := m.Templates[key]
	if !ok {
		return "", ""
	}
	return t.DefName, t.Raw
}
