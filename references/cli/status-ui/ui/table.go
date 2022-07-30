package ui

import (
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
	t.SetBorderPadding(0, 0, 1, 1)

}

func (h *Table) Name() string {
	return "TABLE"
}

func (h *Table) Start() {

}

func (h *Table) Stop() {

}

func (h *Table) Hint() []model.MenuHint {
	return h.actions.Hint()
}

func (t *Table) Actions() model.KeyActions {
	return t.actions
}
