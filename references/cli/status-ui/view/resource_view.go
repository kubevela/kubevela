package view

import (
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type argMap map[string]string

type viewFunc func(app *App, list ResourceList) model.Component
type listFunc func(argMap) ResourceList

type ResourceList interface {
	Header() []string
	Body() [][]string
}

type ResourceViewer struct {
	title    string
	viewFunc viewFunc
	listFunc listFunc
}

type ResourceView struct {
	*Table
	app *App
}

var ResourceMap = map[string]ResourceViewer{
	"app": {
		title:    "Application",
		viewFunc: NewApplicationView,
		listFunc: ListApplications,
	},
	"cluster": {
		title:    "Cluster",
		viewFunc: NewClusterView,
		listFunc: ListClusters,
	},
	"k8s": {
		title:    "K8s-Object",
		viewFunc: NewK8SView,
		listFunc: ListK8SObjects,
	},
}

func NewResourceView(app *App) *ResourceView {
	v := &ResourceView{
		Table: NewTable(app),
		app:   app,
	}
	return v
}

func (v *ResourceView) Init(list ResourceList) {
	v.SetSelectable(true, false)
	v.buildTable(list)
	v.bindKeys()
}

func (v *ResourceView) buildTable(list ResourceList) {
	v.Table.Init()
	v.buildTableHeader(list.Header())
	v.buildTableBody(list.Body())
}

func (v *ResourceView) buildTableHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i])
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

func (v *ResourceView) buildTableBody(body [][]string) {
	for i := 0; i < len(body); i++ {
		for j := 0; j < len(body[i]); j++ {
			c := tview.NewTableCell(body[i][j])
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
