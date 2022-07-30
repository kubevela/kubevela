package view

import (
	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type Application struct {
	name       string
	namespace  string
	alias      string
	createTime string
	updateTime string
	status     string
}

type ApplicationList struct {
	title []string
	data  []Application
}

type ApplicationView struct {
	*ResourceView
	list ResourceList
}

func NewApplicationView(app *App, list ResourceList) model.Component {
	v := &ApplicationView{
		ResourceView: NewResourceView(app),
		list:         list,
	}
	return v
}

func ListApplications(args argMap) ResourceList {
	list := &ApplicationList{
		title: []string{"name", "namespace", "alias", "createTime", "updateTime", "status"},
		data: []Application{{
			"test", "vela", "test", "2022-1-1", "2022-1-2", "running",
		}, {
			"test", "vela", "test", "2022-1-2", "2022-1-3", "running",
		},
		},
	}
	return list
}

func (l *ApplicationList) Header() []string {
	return l.title
}

func (l *ApplicationList) Body() [][]string {
	data := make([][]string, 0)
	for _, app := range l.data {
		data = append(data, []string{app.name, app.namespace, app.alias, app.createTime, app.updateTime, app.status})
	}
	return data
}

func (v *ApplicationView) Init() {
	v.ResourceView.Init(v.list)
	v.bindKeys()
}

func (v *ApplicationView) Name() string {
	return "application"
}

func (v *ApplicationView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *ApplicationView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyEnter: model.KeyAction{Description: "Goto", Action: v.clusterView, Visible: true, Shared: true},
		tcell.KeyESC:   model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:     model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}

func (v *ResourceView) clusterView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	name, namespace := v.GetCell(row, 0), v.GetCell(row, 1)

	args := make(argMap)
	args["name"] = name.Text
	args["namespace"] = namespace.Text

	v.app.command.run("cluster", args)

	return event
}
