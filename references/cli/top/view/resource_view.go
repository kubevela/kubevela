/*
Copyright 2022 The KubeVela Authors.

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

package view

import (
	"context"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ResourceViewFactory product resource view
type ResourceViewFactory interface {
	model.Component
	InitView(ctx context.Context, app *App)
	Update()
	BuildHeader()
	BuildBody()
}

// ResourceViewMap is a map from resource name to resource view
var ResourceViewMap = map[string]ResourceViewFactory{
	"app":      new(ApplicationView),
	"cluster":  new(ClusterView),
	"resource": new(ManagedResourceView),
	"ns":       new(NamespaceView),
	"cns":      new(ClusterNamespaceView),
	"pod":      new(PodView),
}

// ResourceView is an abstract of resource view
type ResourceView struct {
	*component.Table
	app *App
}

// NewResourceView return a new resource view
func NewResourceView(app *App) *ResourceView {
	v := &ResourceView{
		Table: component.NewTable(),
		app:   app,
	}
	return v
}

// Init the resource view
func (v *ResourceView) Init() {
	v.Table.Init()
	v.SetBorder(true)
	v.SetTitleColor(config.ResourceTableTitleColor)
	v.SetSelectable(true, false)
}

// Name return the name of view
func (v *ResourceView) Name() string {
	return "Resource"
}

func (v *ResourceView) BuildHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i])
		c.SetTextColor(config.ResourceTableHeaderColor)
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

func (v *ResourceView) BuildBody(body [][]string) {
	rowNum := len(body)
	for i := 0; i < rowNum; i++ {
		columnNum := len(body[i])
		for j := 0; j < columnNum; j++ {
			c := tview.NewTableCell(body[i][j])
			c.SetTextColor(config.ResourceTableBodyColor)
			c.SetExpansion(3)
			v.SetCell(i+1, j, c)
		}
	}
}
