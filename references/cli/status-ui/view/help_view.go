package view

import (
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type HelpView struct {
	*Table
	app *App
}

func NewHelpView(app *App) *HelpView {
	v := &HelpView{
		Table: NewTable(app),
		app:   app,
	}
	return v
}

func (v *HelpView) Init() {
	v.Table.Init()
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(ui.RESOURCE_TABLE_TITLE_COLOR)
	v.bindKeys()
}

func (v *HelpView) bindKeys() {
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
	})
}

func (v *HelpView) Name() string {
	return "Help"
}
