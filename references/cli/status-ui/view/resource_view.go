package view

import (
	"context"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type ResourceView struct {
	*Table
	app *App
}

type ResourceViewer struct {
	title    string
	viewFunc func(context.Context, *App) model.Component
}

var ResourceMap = map[string]ResourceViewer{
	"app": {
		title:    "Application",
		viewFunc: NewApplicationView,
	},
	"cluster": {
		title:    "Cluster",
		viewFunc: NewClusterView,
	},
	"k8s": {
		title:    "K8s-Object",
		viewFunc: NewK8SView,
	},
}

func NewResourceView(app *App) *ResourceView {
	v := &ResourceView{
		Table: NewTable(app),
		app:   app,
	}
	return v
}

func (v *ResourceView) Init(list model.ResourceList) {
	v.SetSelectable(true, false)
	// record which columns are status column whose color need to specially set
	v.buildTable(list)
	v.bindKeys()
}

func (v *ResourceView) buildTable(list model.ResourceList) {
	v.Table.Init()
	v.buildTableHeader(list.Header())
	v.buildTableBody(list.Body())
}

func (v *ResourceView) buildTableHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i]).SetTextColor(ui.RESOURCE_TABLE_HEADER_COLOR)
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

func (v *ResourceView) buildTableBody(body [][]string) {
	for i := 0; i < len(body); i++ {
		for j := 0; j < len(body[i]); j++ {
			c := tview.NewTableCell(body[i][j])
			c.SetTextColor(ui.RESOURCE_TABLE_BODY_COLOR)
			c.SetExpansion(3)
			v.SetCell(i+1, j, c)
		}
	}
}

func (v *ResourceView) Name() string {
	return ""
}

func (v *ResourceView) Start() {
}

func (v *ResourceView) Stop() {
}
