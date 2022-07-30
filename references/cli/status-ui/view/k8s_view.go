package view

import (
	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type K8SObject struct {
	name       string
	namespace  string
	kind       string
	apiVersion string
	cluster    string
	status     string
}

type K8SObjectList struct {
	title []string
	data  []K8SObject
}

type K8SView struct {
	*ResourceView
	list ResourceList
}

func NewK8SView(app *App, list ResourceList) model.Component {
	v := &K8SView{
		ResourceView: NewResourceView(app),
		list:         list,
	}
	return v
}

func ListK8SObjects(args argMap) ResourceList {
	list := &K8SObjectList{
		title: []string{"name", "namespace", "kind", "APIVersion", "cluster", "status"},
		data: []K8SObject{{
			"configMap", "vela", "configMap", "v1", "hangzhou", "running",
		}, {
			"pod", "vela", "pod", "v1", "hangzhou", "running",
		},
		},
	}
	return list
}

func (l *K8SObjectList) Header() []string {
	return l.title
}

func (l *K8SObjectList) Body() [][]string {
	data := make([][]string, 0)
	for _, app := range l.data {
		data = append(data, []string{app.name, app.namespace, app.kind, app.apiVersion, app.cluster, app.status})
	}
	return data
}

func (v *K8SView) Init() {
	v.ResourceView.Init(v.list)
	v.bindKeys()
}

func (v *K8SView) Name() string {
	return "object"
}

func (v *K8SView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *K8SView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
