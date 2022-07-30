package view

import (
	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type Table struct {
	*ui.Table
	app *App
}

func NewTable(app *App) *Table {
	t := &Table{
		Table: ui.NewTable(),
		app:   app,
	}
	return t
}

func (t *Table) Init() {
	t.Table.Init()
	t.SetInputCapture(t.keyboard)
	t.bindKeys()
}

func (t *Table) bindKeys() {
}

func (t *Table) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}

	if a, ok := t.Actions()[tcell.Key(event.Rune())]; ok {
		return a.Action(event)
	}

	return event
}
