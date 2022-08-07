package component

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Table is a base table component which can be reused by other component
type Table struct {
	*tview.Table
	actions model.KeyActions
}

// NewTable return a new table component
func NewTable() *Table {
	return &Table{
		Table:   tview.NewTable(),
		actions: make(model.KeyActions),
	}
}

// Init table component
func (t *Table) Init() {
	t.SetBorder(true)
	t.SetBorderAttributes(tcell.AttrItalic)
	t.SetBorderPadding(1, 1, 1, 1)
}

// Name return table's name
func (t *Table) Name() string {
	return "table"
}

// Start table component
func (t *Table) Start() {
}

// Stop table component
func (t *Table) Stop() {
}

// Hint return key action menu hints of the component
func (t *Table) Hint() []model.MenuHint {
	return t.actions.Hint()
}

// Actions return actions
func (t *Table) Actions() model.KeyActions {
	return t.actions
}
