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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ResourceView is the interface to abstract resource view
type ResourceView interface {
	model.View
	InitView(ctx context.Context, app *App)
	Refresh(event *tcell.EventKey) *tcell.EventKey
	Update()
	BuildHeader()
	BuildBody()
}

// ResourceViewMap is a map from resource name to resource view
var ResourceViewMap = map[string]ResourceView{
	"app":      new(ApplicationView),
	"cluster":  new(ClusterView),
	"resource": new(ManagedResourceView),
	"ns":       new(NamespaceView),
	"cns":      new(ClusterNamespaceView),
	"pod":      new(PodView),
}

// CommonResourceView is an abstract of resource view
type CommonResourceView struct {
	*component.Table
	app *App
}

// NewCommonView return a new common view
func NewCommonView(app *App) *CommonResourceView {
	return &CommonResourceView{
		Table: component.NewTable(),
		app:   app,
	}
}

// Init the common resource view
func (v *CommonResourceView) Init() {
	v.Table.Init()
	v.SetBorder(true)
	v.SetTitleColor(config.ResourceTableTitleColor)
	v.SetSelectable(true, false)
}

// Name return the name of common view
func (v *CommonResourceView) Name() string {
	return "Resource"
}

// BuildHeader render the header of table
func (v *CommonResourceView) BuildHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i])
		c.SetTextColor(config.ResourceTableHeaderColor)
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

// BuildBody render the body of table
func (v *CommonResourceView) BuildBody(body [][]string) {
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

// Refresh the base resource view
func (v *CommonResourceView) Refresh(update func()) {
	v.Clear()
	v.app.QueueUpdateDraw(update)
}
