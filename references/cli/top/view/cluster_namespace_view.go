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

// ClusterNamespaceView is the cluster namespace, which display the namespace info of application's resource
type ClusterNamespaceView struct {
	*ResourceView
	ctx context.Context
}

// Name return cluster namespace view name
func (v *ClusterNamespaceView) Name() string {
	return "ClusterNamespace"
}

// Init the cluster namespace view
func (v *ClusterNamespaceView) Init() {
	v.ResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Name())).SetTitleColor(config.ResourceTableTitleColor)
	v.BuildHeader()
	v.bindKeys()
}

// Start the cluster namespace view
func (v *ClusterNamespaceView) Start() {
	v.Update()
}

// Stop the cluster namespace view
func (v *ClusterNamespaceView) Stop() {
	v.Table.Stop()
}

// Hint return key action menu hints of the cluster namespace view
func (v *ClusterNamespaceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *ClusterNamespaceView) InitView(ctx context.Context, app *App) {
	if v.ResourceView == nil {
		v.ResourceView = NewResourceView(app)
		v.ctx = ctx
	} else {
		v.ctx = ctx
	}
}

// Update refresh the content of body of view
func (v *ClusterNamespaceView) Update() {
	v.BuildBody()
}

// BuildHeader render the header of table
func (v *ClusterNamespaceView) BuildHeader() {
	header := []string{"Name", "Status", "Age"}
	v.ResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ClusterNamespaceView) BuildBody() {
	cnList, err := model.ListClusterNamespaces(v.ctx, v.app.client)
	if err != nil {
		return
	}
	cnInfos := cnList.ToTableBody()
	v.ResourceView.BuildBody(cnInfos)
	rowNum := len(cnInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *ClusterNamespaceView) ColorizeStatusText(rowNum int) {
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

func (v *ClusterNamespaceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Select", Action: v.managedResourceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// managedResourceView switch cluster namespace view to managed resource view
func (v *ClusterNamespaceView) managedResourceView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}

	clusterNamespace := v.GetCell(row, 0).Text
	if clusterNamespace == model.AllClusterNamespace {
		clusterNamespace = ""
	}
	v.app.content.PopComponent()
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyClusterNamespace, clusterNamespace)
	v.app.command.run(v.ctx, "resource")
	return event
}
