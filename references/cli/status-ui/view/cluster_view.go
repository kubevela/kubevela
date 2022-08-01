package view

import (
	"context"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type ClusterView struct {
	*ResourceView
	ctx context.Context
}

func NewClusterView(ctx context.Context, app *App) model.Component {
	v := &ClusterView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

func (v *ClusterView) Init() {
	v.SetTitle(v.Name())
	resourceList := v.ListClusters()
	v.ResourceView.Init(resourceList)
	v.bindKeys()
}

func (v *ClusterView) ListClusters() model.ResourceList {
	list := model.ListClusters(v.ctx, v.app.client)
	return list
}

func (v *ClusterView) Name() string {
	return "cluster"
}

func (v *ClusterView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *ClusterView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter: model.KeyAction{Description: "Goto", Action: v.k8sObjectView, Visible: true, Shared: true},
		tcell.KeyESC:   model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:     model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

func (v *ClusterView) k8sObjectView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	clusterName := v.GetCell(row, 0).Text
	v.ctx = context.WithValue(v.ctx, "cluster", clusterName)
	v.app.command.run(v.ctx, "k8s")
	return event
}
