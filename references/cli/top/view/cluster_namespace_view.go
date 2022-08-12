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

// ClusterNamespaceView is the cluster namespace, which display the namespace info of application's resource
type ClusterNamespaceView struct {
	*ResourceView
	ctx context.Context
}

// NewClusterNamespaceView return a new cluster namespace view
func NewClusterNamespaceView(ctx context.Context, app *App) model.Component {
	v := &ClusterNamespaceView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init the cluster namespace view
func (v *ClusterNamespaceView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListClusterNamespaces()
	v.ResourceView.Init(resourceList)

	v.bindKeys()
}

// ListClusterNamespaces return the namespace of application's resource
func (v *ClusterNamespaceView) ListClusterNamespaces() model.ResourceList {
	return model.ListClusterNamespaces(v.ctx, v.app.client)
}

// Hint return key action menu hints of the cluster namespace view
func (v *ClusterNamespaceView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// Name return cluster namespace view name
func (v *ClusterNamespaceView) Name() string {
	return "ClusterNamespace"
}

func (v *ClusterNamespaceView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter:    model.KeyAction{Description: "Select", Action: v.k8sObjectView, Visible: true, Shared: true},
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

// k8sObjectView switch cluster namespace view to k8s object view
func (v *ClusterNamespaceView) k8sObjectView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	v.app.content.PopComponent()
	clusterNamespace := v.GetCell(row, 0).Text
	if clusterNamespace == model.AllClusterNamespace {
		clusterNamespace = ""
	}
	v.ctx = context.WithValue(v.ctx, &model.CtxKeyClusterNamespace, clusterNamespace)
	v.app.command.run(v.ctx, "k8s")
	return event
}
