package view

import (
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

func (h *HelpView) Init() {
	h.Table.Init()
	h.SetTitle(h.Name())
	h.bindKeys()
}

func (h *HelpView) bindKeys() {
	h.Actions().Add(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: h.app.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Back", Action: h.app.Back, Visible: true, Shared: true},
	})
}

func (h *HelpView) Name() string {
	return "Help"
}
