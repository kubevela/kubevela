package view

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type ApplicationView struct {
	*ResourceView
	ctx context.Context
}

func NewApplicationView(ctx context.Context, app *App) model.Component {
	v := &ApplicationView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

func (v *ApplicationView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(ui.RESOURCE_TABLE_TITLE_COLOR)
	resourceList := v.ListApplications()
	v.ResourceView.Init(resourceList)
	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

func (v *ApplicationView) ListApplications() model.ResourceList {
	appList, err := model.ListApplications(context.Background(), v.app.client)
	if err != nil {
		return appList
	}
	return appList
}

func (v *ApplicationView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 2).Text
		if status == "running" {
			status = fmt.Sprintf("[lightgreen::]%s", status)
		}
		v.Table.GetCell(i, 2).SetText(status)
	}
}

func (v *ApplicationView) Name() string {
	return "Application"
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

func (v *ApplicationView) clusterView(event *tcell.EventKey) *tcell.EventKey {
	row, _ := v.GetSelection()
	if row == 0 {
		return event
	}
	name, namespace := v.GetCell(row, 0).Text, v.GetCell(row, 1).Text

	v.ctx = context.WithValue(v.ctx, "appName", name)
	v.ctx = context.WithValue(v.ctx, "appNs", namespace)

	v.app.command.run(v.ctx, "cluster")
	return event
}
