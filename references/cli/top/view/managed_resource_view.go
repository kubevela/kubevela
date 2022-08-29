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
	"log"

	"github.com/gdamore/tcell/v2"

	querytypes "github.com/oam-dev/kubevela/pkg/velaql/providers/query/types"
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ManagedResourceView is a view which displays info of application's managed resource including CRDs and k8s objects
type ManagedResourceView struct {
	*ResourceView
	ctx context.Context
}

// NewManagedResourceView return a new managed resource view
func NewManagedResourceView(ctx context.Context, app *App) model.Component {
	v := &ManagedResourceView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init managed resource view
func (v *ManagedResourceView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Title())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListManagedResources()
	v.ResourceView.Init(resourceList)

	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

// ListManagedResources return managed resource of the aimed application
func (v *ManagedResourceView) ListManagedResources() model.ResourceList {
	list, err := model.ListManagedResource(v.ctx, v.app.client)
	if err != nil {
		log.Println(err)
	}
	return list
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

// Name return managed resource view name
func (v *ManagedResourceView) Name() string {
	return "Managed Resource"
}

// Hint return key action menu hints of the managed resource view
func (v *ManagedResourceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// ColorizeStatusText colorize the status column text
func (v *ManagedResourceView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 5).Text
		switch querytypes.HealthStatusCode(status) {
		case querytypes.HealthStatusHealthy:
			status = config.ObjectHealthyStatusColor + status
		case querytypes.HealthStatusUnHealthy:
			status = config.ObjectUnhealthyStatusColor + status
		case querytypes.HealthStatusProgressing:
			status = config.ObjectProgressingStatusColor + status
		case querytypes.HealthStatusUnKnown:
			status = config.ObjectUnKnownStatusColor + status
		default:
		}
		v.Table.GetCell(i, 5).SetText(status)
	}
}

func (v *ManagedResourceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Enter", Action: v.podView, Visible: true, Shared: true},
		component.KeyC:    model.KeyAction{Description: "Select Cluster", Action: v.clusterView, Visible: true, Shared: true},
		component.KeyN:    model.KeyAction{Description: "Select ClusterNS", Action: v.clusterNamespaceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// clusterView switch managed resource view to the cluster view
func (v *ManagedResourceView) clusterView(event *tcell.EventKey) *tcell.EventKey {
	v.app.content.PopComponent()
	v.app.command.run(v.ctx, "cluster")
	return event
}

// clusterView switch managed resource view to the cluster Namespace view
func (v *ManagedResourceView) clusterNamespaceView(event *tcell.EventKey) *tcell.EventKey {
	v.app.content.PopComponent()
	v.app.command.run(v.ctx, "cns")
	return event
}

func (v *ManagedResourceView) podView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace, cluster := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text, v.GetCell(row, 4).Text
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyCluster, cluster)
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyClusterNamespace, namespace)
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyComponentName, name)

	v.app.command.run(v.ctx, "pod")
	return nil
}
