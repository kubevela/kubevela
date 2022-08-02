package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Table struct {
	*tview.Table
	actions model.KeyActions
}

func NewTable() *Table {
	return &Table{
		Table:   tview.NewTable(),
		actions: make(model.KeyActions),
	}
}

func (t *Table) Init() {
	t.SetBorder(true)
	t.SetBorderAttributes(tcell.AttrItalic)
	t.SetBorderPadding(1, 1, 1, 1)
}

func (t *Table) Start() {
}

func (t *Table) Stop() {
}

func (t *Table) Hint() []model.MenuHint {
	return t.actions.Hint()
}

func (t *Table) Actions() model.KeyActions {
	return t.actions
}
