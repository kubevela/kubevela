package view

import (
	"context"

	"github.com/oam-dev/kubevela/references/cli/top/config"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// ResourceView is an abstract of resource view
type ResourceView struct {
	*Table
	app *App
}

// ResourceViewer is resource's renderer
type ResourceViewer struct {
	viewFunc func(context.Context, *App) model.Component
}

// ResourceMap is a map from resource name to resource's renderer
var ResourceMap = map[string]ResourceViewer{
	"app": {
		viewFunc: NewApplicationView,
	},
	"cluster": {
		viewFunc: NewClusterView,
	},
	"k8s": {
		viewFunc: NewK8SView,
	},
}

// NewResourceView return a new resource view
func NewResourceView(app *App) *ResourceView {
	v := &ResourceView{
		Table: NewTable(),
		app:   app,
	}
	return v
}

// Init the resource view
func (v *ResourceView) Init(list model.ResourceList) {
	v.SetSelectable(true, false)
	v.buildTable(list)
}

func (v *ResourceView) buildTable(list model.ResourceList) {
	v.Table.Init()
	v.buildTableHeader(list.Header())
	v.buildTableBody(list.Body())
}

// buildTableHeader render the resource table header
func (v *ResourceView) buildTableHeader(header []string) {
	for i := 0; i < len(header); i++ {
		c := tview.NewTableCell(header[i]).SetTextColor(config.ResourceTableHeaderColor)
		c.SetExpansion(3)
		v.SetCell(0, i, c)
	}
}

// buildTableBody render the resource table body
func (v *ResourceView) buildTableBody(body [][]string) {
	for i := 0; i < len(body); i++ {
		for j := 0; j < len(body[i]); j++ {
			c := tview.NewTableCell(body[i][j])
			c.SetTextColor(config.ResourceTableBodyColor)
			c.SetExpansion(3)
			v.SetCell(i+1, j, c)
		}
	}
}

// Name return view name
func (v *ResourceView) Name() string {
	return ""
}
