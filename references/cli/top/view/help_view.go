package view

import (
	"fmt"

	"github.com/oam-dev/kubevela/references/cli/top/config"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// HelpView is the view which display help tips about how to use app
type HelpView struct {
	*Table
	app *App
}

// NewHelpView return a new help view
func NewHelpView(app *App) *HelpView {
	v := &HelpView{
		Table: NewTable(),
		app:   app,
	}
	return v
}

// Init help view init
func (v *HelpView) Init() {
	v.Table.Init()
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	v.bindKeys()
}

func (v *HelpView) bindKeys() {
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
	})
}

// Name return help view name
func (v *HelpView) Name() string {
	return "Help"
}
