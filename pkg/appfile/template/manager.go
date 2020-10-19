package template

import (
	"github.com/oam-dev/kubevela/api/types"
	"github.com/oam-dev/kubevela/pkg/plugins"
)

type Manager interface {
	IsTrait(key string) bool
	LoadTemplate(key string) string
}

func Load() (Manager, error) {
	caps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return nil, err
	}
	m := newManager()
	for _, cap := range caps {
		t := &template{}
		t.captype = cap.Type
		t.raw = cap.CueTemplate
		m.templates[cap.Name] = t
	}
	return m, nil
}

type manager struct {
	templates map[string]*template
}

func newManager() *manager {
	return &manager{
		templates: make(map[string]*template),
	}
}

type template struct {
	captype types.CapType
	raw     string
}

func (m *manager) IsTrait(key string) bool {
	t, ok := m.templates[key]
	if !ok {
		return false
	}
	return t.captype == types.TypeTrait
}

func (m *manager) LoadTemplate(key string) string {
	t, ok := m.templates[key]
	if !ok {
		return ""
	}
	return t.raw
}
