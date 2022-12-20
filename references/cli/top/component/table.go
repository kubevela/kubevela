/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package component

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Table is a base table component which can be reused by other component
type Table struct {
	*tview.Table
	style   *config.ThemeConfig
	actions model.KeyActions
}

// NewTable return a new table component
func NewTable(style *config.ThemeConfig) *Table {
	return &Table{
		Table:   tview.NewTable(),
		style:   style,
		actions: make(model.KeyActions),
	}
}

// Init table component
func (t *Table) Init() {
	t.SetBorderAttributes(tcell.AttrItalic)
	t.SetBorder(true)
	t.SetBorderColor(t.style.Border.Table.Color())
	t.SetBorderPadding(1, 1, 1, 1)
	t.SetInputCapture(t.keyboard)
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
	t.Clear()
}

// Hint return key action menu hints of the component
func (t *Table) Hint() []model.MenuHint {
	return t.actions.Hint()
}

// Actions return actions
func (t *Table) Actions() model.KeyActions {
	return t.actions
}

func (t *Table) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}
	if a, ok := t.Actions()[StandardizeKey(event)]; ok {
		return a.Action(event)
	}
	return event
}
