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
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ClusterNamespaceView is the cluster namespace, which display the namespace info of application's resource
type ClusterNamespaceView struct {
	*CommonResourceView
	ctx context.Context
}

// Name return cluster namespace view name
func (v *ClusterNamespaceView) Name() string {
	return "ClusterNamespace"
}

// Init the cluster namespace view
func (v *ClusterNamespaceView) Init() {
	v.CommonResourceView.Init()
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Name())).SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
}

// Start the cluster namespace view
func (v *ClusterNamespaceView) Start() {
	v.Clear()
	v.Update(func() {})
	v.AutoRefresh(v.Update)
}

// Stop the cluster namespace view
func (v *ClusterNamespaceView) Stop() {
	v.CommonResourceView.Stop()
}

// Hint return key action menu hints of the cluster namespace view
func (v *ClusterNamespaceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// InitView init a new cluster namespace view
func (v *ClusterNamespaceView) InitView(ctx context.Context, app *App) {
	v.ctx = ctx
	if v.CommonResourceView == nil {
		v.CommonResourceView = NewCommonView(app)
	}
}

// Refresh the view content
func (v *ClusterNamespaceView) Refresh(_ *tcell.EventKey) *tcell.EventKey {
	v.CommonResourceView.Refresh(true, v.Update)
	return nil
}

// Update refresh the content of body of view
func (v *ClusterNamespaceView) Update(timeoutCancel func()) {
	v.BuildHeader()
	v.BuildBody()
	timeoutCancel()
}

// BuildHeader render the header of table
func (v *ClusterNamespaceView) BuildHeader() {
	header := []string{"Name", "Status", "Age"}
	v.CommonResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ClusterNamespaceView) BuildBody() {
	cnList, err := model.ListClusterNamespaces(v.ctx, v.app.client)
	if err != nil {
		return
	}
	cnInfos := cnList.ToTableBody()
	v.CommonResourceView.BuildBody(cnInfos)
	rowNum := len(cnInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *ClusterNamespaceView) ColorizeStatusText(rowNum int) {
	for i := 0; i < rowNum; i++ {
		status := v.Table.GetCell(i+1, 1).Text
		highlightColor := v.app.config.Theme.Table.Body.String()
		switch v1.NamespacePhase(status) {
		case v1.NamespaceActive:
			highlightColor = v.app.config.Theme.Status.Healthy.String()
		case v1.NamespaceTerminating:
			highlightColor = v.app.config.Theme.Status.UnHealthy.String()
		}
		v.Table.GetCell(i+1, 1).SetText(fmt.Sprintf("[%s::]%s", highlightColor, status))
	}
}

func (v *ClusterNamespaceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter: model.KeyAction{Description: "Select", Action: v.managedResourceView, Visible: true, Shared: true},
		component.KeyR: model.KeyAction{Description: "Refresh", Action: v.Refresh, Visible: true, Shared: true},
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
	v.app.content.PopView()
	v.app.content.PopView()
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyClusterNamespace, clusterNamespace)
	v.app.command.run(v.ctx, "resource")
	return event
}
