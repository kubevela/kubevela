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

// NewClusterView return a new cluster view
func NewClusterView(ctx context.Context, app *App) model.Component {
	v := &ClusterView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init cluster view init
func (v *ClusterView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListClusters()
	v.ResourceView.Init(resourceList)
	v.bindKeys()
}

// ListClusters list clusters where application deployed
func (v *ClusterView) ListClusters() model.ResourceList {
	return model.ListClusters(v.ctx, v.app.client)
}

// Name return cluster view name
func (v *ClusterView) Name() string {
	return "Cluster"
}

// Hint return key action menu hints of the cluster view
func (v *ClusterView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *ClusterView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Goto", Action: v.k8sObjectView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// k8sObjectView switch app main view to k8s object view
func (v *ClusterView) k8sObjectView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	clusterName := v.GetCell(row, 0).Text
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyCluster, clusterName)
	v.app.command.run(v.ctx, "k8s")
	return event
}
