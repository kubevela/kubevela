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
	"fmt"

	"github.com/gdamore/tcell/v2"
	v1 "k8s.io/api/core/v1"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// NamespaceView is namespace view struct
type NamespaceView struct {
	*ResourceView
	ctx context.Context
}

// NewNamespaceView return a new namespace view
func NewNamespaceView(ctx context.Context, app *App) model.Component {
	v := &NamespaceView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init a namespace view
func (v *NamespaceView) Init() {
	v.ResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Name())).SetTitleColor(config.ResourceTableTitleColor)
	v.BuildHeader()
	v.bindKeys()
}

// Name return k8s view name
func (v *NamespaceView) Name() string {
	return "Namespace"
}

// Start the managed namespace view
func (v *NamespaceView) Start() {
	v.Update()
}

// Stop the managed namespace view
func (v *NamespaceView) Stop() {
	v.Table.Stop()
}

// Hint return key action menu hints of the k8s view
func (v *NamespaceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *NamespaceView) InitView(ctx context.Context, app *App) {
	if v.ResourceView == nil {
		v.ResourceView = NewResourceView(app)
		v.ctx = ctx
	} else {
		v.ctx = ctx
	}
}

// Update refresh the content of body of view
func (v *NamespaceView) Update() {
	v.BuildBody()
}

// BuildHeader render the header of table
func (v *NamespaceView) BuildHeader() {
	header := []string{"Name", "Status", "Age"}
	v.ResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *NamespaceView) BuildBody() {
	nsList, err := model.ListNamespaces(v.ctx, v.app.client)
	if err != nil {
		return
	}
	nsInfos := nsList.ToTableBody()
	v.ResourceView.BuildBody(nsInfos)
	rowNum := len(nsInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *NamespaceView) ColorizeStatusText(rowNum int) {
	for i := 0; i < rowNum; i++ {
		status := v.Table.GetCell(i+1, 2).Text
		switch v1.NamespacePhase(status) {
		case v1.NamespaceActive:
			status = config.NamespaceActiveStatusColor + status
		case v1.NamespaceTerminating:
			status = config.NamespaceTerminateStatusColor + status
		}
		v.Table.GetCell(i+1, 2).SetText(status)
	}
}

func (v *NamespaceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Select", Action: v.applicationView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

func (v *NamespaceView) applicationView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	ns := v.Table.GetCell(row, 0).Text
	if ns == model.AllNamespace {
		ns = ""
	}
	v.app.content.PopComponent()
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyNamespace, ns)
	v.app.command.run(v.ctx, "app")
	return event
}
