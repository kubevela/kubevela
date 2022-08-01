package view

import (
	"context"

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
	resourceList := v.ListK8SObjects()
	v.ResourceView.Init(resourceList)
	v.ResourceView.Init(resourceList)
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

func (v *K8SView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
