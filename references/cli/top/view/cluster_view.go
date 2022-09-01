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
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ClusterView is the cluster view, this view display info of cluster where selected application deployed
type ClusterView struct {
	*ResourceView
	ctx context.Context
}

// Init cluster view init
func (v *ClusterView) Init() {
	v.ResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Name())).SetTitleColor(config.ResourceTableTitleColor)
	v.BuildHeader()
	v.bindKeys()
}

// Name return cluster view name
func (v *ClusterView) Name() string {
	return "Cluster"
}

// Start the cluster view
func (v *ClusterView) Start() {
	v.Update()
}

// Stop the cluster view
func (v *ClusterView) Stop() {
	v.Table.Stop()
}

// Hint return key action menu hints of the cluster view
func (v *ClusterView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// InitView init a new cluster view
func (v *ClusterView) InitView(ctx context.Context, app *App) {
	if v.ResourceView == nil {
		v.ResourceView = NewResourceView(app)
		v.ctx = ctx
	} else {
		v.ctx = ctx
	}
}

// Update refresh the content of body of view
func (v *ClusterView) Update() {
	v.BuildBody()
}

// BuildHeader render the header of table
func (v *ClusterView) BuildHeader() {
	header := []string{"Name", "Alias", "Type", "EndPoint", "Labels"}
	v.ResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ClusterView) BuildBody() {
	clusterList, err := model.ListClusters(v.ctx, v.app.client)
	if err != nil {
		return
	}
	clusterInfos := clusterList.ToTableBody()
	v.ResourceView.BuildBody(clusterInfos)
}

func (v *ClusterView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Goto", Action: v.managedResourceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// managedResourceView switch cluster view to managed resource view
func (v *ClusterView) managedResourceView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	clusterName := v.GetCell(row, 0).Text
	if clusterName == model.AllCluster {
		clusterName = ""
	}
	v.app.content.PopComponent()
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyCluster, clusterName)
	v.app.command.run(v.ctx, "resource")
	return event
}
