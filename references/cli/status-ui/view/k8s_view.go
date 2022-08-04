package view

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type K8SView struct {
	*ResourceView
	ctx context.Context
}

func NewK8SView(ctx context.Context, app *App) model.Component {
	v := &K8SView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

func (v *K8SView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(ui.RESOURCE_TABLE_TITLE_COLOR)
	resourceList := v.ListK8SObjects()
	v.ResourceView.Init(resourceList)
	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

func (v *K8SView) ListK8SObjects() model.ResourceList {
	list := model.ListObjects(v.ctx, v.app.client)
	return list
}

func (v *K8SView) Name() string {
	return "K8S-Object"
}

func (v *K8SView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

func (v *K8SView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 5).Text
		if status == "Healthy" {
			status = fmt.Sprintf("[lightgreen::]%s", status)
		} else if status == "UnHealthy" {
			status = fmt.Sprintf("[red::]%s", status)
		}
		v.Table.GetCell(i, 5).SetText(status)
	}
}

func (v *K8SView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
