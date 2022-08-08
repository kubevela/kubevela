package view

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// K8SView is a view which displays info of kubernetes objects which are generated when application deployed
type K8SView struct {
	*ResourceView
	ctx context.Context
}

// NewK8SView return a new k8s view
func NewK8SView(ctx context.Context, app *App) model.Component {
	v := &K8SView{
		ResourceView: NewResourceView(app),
		ctx:          ctx,
	}
	return v
}

// Init k8s view
func (v *K8SView) Init() {
	// set title of view
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)

	resourceList := v.ListK8SObjects()
	v.ResourceView.Init(resourceList)

	v.ColorizeStatusText(len(resourceList.Body()))
	v.bindKeys()
}

// ListK8SObjects return kubernetes objects of the aimed application
func (v *K8SView) ListK8SObjects() model.ResourceList {
	return model.ListObjects(v.ctx, v.app.client)
}

// Name return k8s view name
func (v *K8SView) Name() string {
	return "K8S-Object"
}

// Hint return key action menu hints of the k8s view
func (v *K8SView) Hint() []model.MenuHint {
	return v.Actions().Hint()
}

// ColorizeStatusText colorize the status column text
func (v *K8SView) ColorizeStatusText(rowNum int) {
	for i := 1; i < rowNum+1; i++ {
		status := v.Table.GetCell(i, 5).Text
		switch status {
		case "Healthy":
			status = fmt.Sprintf("[green::]%s", status)
		case "UnHealthy":
			status = fmt.Sprintf("[red::]%s", status)
		case "Progressing":
			status = fmt.Sprintf("[blue::]%s", status)
		case "UnKnown":
			status = fmt.Sprintf("[gray::]%s", status)
		}
		v.Table.GetCell(i, 5).SetText(status)
	}
}

func (v *K8SView) bindKeys() {
	v.Actions().Delete([]tcell.Key{tcell.KeyEnter})
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
