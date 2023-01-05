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

	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ManagedResourceView is a view which displays info of application's managed resource including CRDs and k8s objects
type ManagedResourceView struct {
	*CommonResourceView
	ctx context.Context
}

// Name return managed resource view name
func (v *ManagedResourceView) Name() string {
	return "Managed Resource"
}

// Init managed resource view
func (v *ManagedResourceView) Init() {
	v.CommonResourceView.Init()
	// set title of view
	v.SetTitle(fmt.Sprintf("[ %s ]", v.Title())).SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
}

// Start the managed resource view
func (v *ManagedResourceView) Start() {
	v.Clear()
	v.Update(func() {})
	v.CommonResourceView.AutoRefresh(v.Update)
}

// Stop the managed resource view
func (v *ManagedResourceView) Stop() {
	v.CommonResourceView.Stop()
}

// Hint return key action menu hints of the managed resource view
func (v *ManagedResourceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// Title return the table title of managed resource view
func (v *ManagedResourceView) Title() string {
	namespace, ok := v.ctx.Value(&model.CtxKeyCluster).(string)
	if !ok || namespace == "" {
		namespace = "all"
	}
	clusterNS, ok := v.ctx.Value(&model.CtxKeyClusterNamespace).(string)
	if !ok || clusterNS == "" {
		clusterNS = "all"
	}
	return fmt.Sprintf("Managed Resource"+" (%s/%s)", namespace, clusterNS)
}

// InitView init a new managed resource view
func (v *ManagedResourceView) InitView(ctx context.Context, app *App) {
	v.ctx = ctx
	if v.CommonResourceView == nil {
		v.CommonResourceView = NewCommonView(app)
	}
}

// Refresh the view content
func (v *ManagedResourceView) Refresh(_ *tcell.EventKey) *tcell.EventKey {
	v.CommonResourceView.Refresh(true, v.Update)
	return nil
}

// Update refresh the content of body of view
func (v *ManagedResourceView) Update(timeoutCancel func()) {
	v.BuildHeader()
	v.BuildBody()
	timeoutCancel()
}

// BuildHeader render the header of table
func (v *ManagedResourceView) BuildHeader() {
	header := []string{"Name", "Namespace", "Kind", "APIVersion", "Cluster", "Component", "Status"}
	v.CommonResourceView.BuildHeader(header)
}

// BuildBody render the body of table
func (v *ManagedResourceView) BuildBody() {
	resourceList, err := model.ListManagedResource(v.ctx, v.app.client)
	if err != nil {
		return
	}
	resourceInfos := resourceList.ToTableBody()
	v.CommonResourceView.BuildBody(resourceInfos)
	rowNum := len(resourceInfos)
	v.ColorizeStatusText(rowNum)
}

// ColorizeStatusText colorize the status column text
func (v *ManagedResourceView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 6).Text
		highlightColor := v.app.config.Theme.Table.Body.String()

		switch querytypes.HealthStatusCode(status) {
		case querytypes.HealthStatusHealthy:
			highlightColor = v.app.config.Theme.Status.Healthy.String()
		case querytypes.HealthStatusUnHealthy:
			highlightColor = v.app.config.Theme.Status.UnHealthy.String()
		case querytypes.HealthStatusProgressing:
			highlightColor = v.app.config.Theme.Status.Waiting.String()
		case querytypes.HealthStatusUnKnown:
			highlightColor = v.app.config.Theme.Status.Unknown.String()
		default:
		}
		v.Table.GetCell(i, 6).SetText(fmt.Sprintf("[%s::]%s", highlightColor, status))
	}
}

func (v *ManagedResourceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter: model.KeyAction{Description: "Pods", Action: v.podView, Visible: true, Shared: true},
		component.KeyC: model.KeyAction{Description: "Select Cluster", Action: v.clusterView, Visible: true, Shared: true},
		component.KeyN: model.KeyAction{Description: "Select ClusterNS", Action: v.clusterNamespaceView, Visible: true, Shared: true},
		component.KeyY: model.KeyAction{Description: "Yaml", Action: v.yamlView, Visible: true, Shared: true},
		component.KeyR: model.KeyAction{Description: "Refresh", Action: v.Refresh, Visible: true, Shared: true},
	})
}

// clusterView switch managed resource view to the cluster view
func (v *ManagedResourceView) clusterView(event *tcell.EventKey) *tcell.EventKey {
	v.app.command.run(v.ctx, "cluster")
	return event
}

// clusterView switch managed resource view to the cluster Namespace view
func (v *ManagedResourceView) clusterNamespaceView(event *tcell.EventKey) *tcell.EventKey {
	v.app.command.run(v.ctx, "cns")
	return event
}

func (v *ManagedResourceView) podView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace, cluster := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text, v.GetCell(row, 4).Text

	ctx := context.WithValue(v.ctx, &model.CtxKeyCluster, cluster)
	ctx = context.WithValue(ctx, &model.CtxKeyClusterNamespace, namespace)
	ctx = context.WithValue(ctx, &model.CtxKeyComponentName, name)
	v.app.command.run(ctx, "pod")
	return nil
}

func (v *ManagedResourceView) yamlView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text
	kind, api, cluster := v.GetCell(row, 2).Text, v.GetCell(row, 3).Text, v.GetCell(row, 4).Text

	gvr := model.GVR{
		GV: api,
		R: model.Resource{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
			Cluster:   cluster,
		},
	}
	ctx := context.WithValue(v.app.ctx, &model.CtxKeyGVR, &gvr)
	v.app.command.run(ctx, "yaml")
	return nil
}
