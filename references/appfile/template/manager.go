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

package template

import (
	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/common"
	"github.com/oam-dev/kubevela/references/docgen"
)

// Manager defines a manager for template
type Manager interface {
	IsTrait(key string) bool
	LoadTemplate(key string) (tmpl string)
}

// Load will load all installed capabilities and create a manager
func Load(namespace string, c common.Args) (Manager, error) {
	caps, err := docgen.LoadAllInstalledCapability(namespace, c)
	if err != nil {
		return nil, err
	}
	m := newManager()
	for _, cap := range caps {
		t := &Template{}
		t.Captype = cap.Type
		t.Raw = cap.CueTemplate
		m.Templates[cap.Name] = t
	}
	return m, nil
}

// Template defines a raw template struct
type Template struct {
	Captype types.CapType
	Raw     string
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

func (m *manager) LoadTemplate(key string) string {
	t, ok := m.Templates[key]
	if !ok {
		return ""
	}
	return t.Raw
}
