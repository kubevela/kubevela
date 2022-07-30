package view

import (
	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type ClusterView struct {
	*ResourceView
	list ResourceList
}

type Cluster struct {
	name        string
	description string
	status      string
}

type ClusterList struct {
	title []string
	data  []Cluster
}

func NewClusterView(app *App, list ResourceList) model.Component {
	v := &ClusterView{
		ResourceView: NewResourceView(app),
		list:         list,
	}
	return v
}

func ListClusters(args argMap) ResourceList {
	list := &ClusterList{
		title: []string{"name", "description", "status"},
		data: []Cluster{{
			"hangzhou", "hangzhou", "running",
		}, {
			"beijing", "beijing", "running",
		},
		},
	}
	return list
}

func (l *ClusterList) Header() []string {
	return l.title
}

func (l *ClusterList) Body() [][]string {
	data := make([][]string, 0)
	for _, cluster := range l.data {
		data = append(data, []string{cluster.name, cluster.description, cluster.status})
	}
	return data
}

func (v *ClusterView) Init() {
	v.ResourceView.Init(v.list)
	v.bindKeys()
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
	name := v.GetCell(row, 0)

	args := make(argMap)
	args["name"] = name.Text

	v.app.command.run("k8s", args)
	return event
}
