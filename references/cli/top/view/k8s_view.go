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
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// K8SView is a view which displays info of kubernetes objects which are generated when application deployed
type K8SView struct {
	*ResourceView
	ctx context.Context
}

// NewK8SView return a new k8s view
func NewK8SView(ctx context.Context, app *App) model.Component {
	v := &K8SView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init k8s view
func (v *K8SView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Title())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListK8SObjects()
	v.ResourceView.Init(resourceList)

	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

// ListK8SObjects return kubernetes objects of the aimed application
func (v *K8SView) ListK8SObjects() model.ResourceList {
	return model.ListObjects(v.ctx, v.app.client)
}

// Title return the table title of k8s object view
func (v *K8SView) Title() string {
	namespace, ok := v.ctx.Value(&model.CtxKeyCluster).(string)
	if !ok || namespace == "" {
		namespace = "all"
	}
	clusterNS, ok := v.ctx.Value(&model.CtxKeyClusterNamespace).(string)
	if !ok || clusterNS == "" {
		clusterNS = "all"
	}
	return fmt.Sprintf("K8S-Object"+" (%s/%s)", namespace, clusterNS)
}

// Name return k8s view name
func (v *K8SView) Name() string {
	return "K8S-Object"
}

// Hint return key action menu hints of the k8s view
func (v *K8SView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// ColorizeStatusText colorize the status column text
func (v *K8SView) ColorizeStatusText(rowNum int) {
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

func (v *K8SView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		component.KeyC:    model.KeyAction{Description: "Select Cluster", Action: v.clusterView, Visible: true, Shared: true},
		component.KeyN:    model.KeyAction{Description: "Select ClusterNS", Action: v.clusterNamespaceView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// clusterView switch k8s object view to the cluster view
func (v *K8SView) clusterView(event *tcell.EventKey) *tcell.EventKey {
	v.app.content.PopComponent()
	v.app.command.run(v.ctx, "cluster")
	return event
}

// clusterView switch k8s object view to the cluster Namespace view
func (v *K8SView) clusterNamespaceView(event *tcell.EventKey) *tcell.EventKey {
	v.app.content.PopComponent()
	v.app.command.run(v.ctx, "cns")
	return event
}
