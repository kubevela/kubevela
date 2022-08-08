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

package view

import (
	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/top/component"
)

// Table is a base table view, help view and resource view all base on it
type Table struct {
	*component.Table
}

// NewTable return a new table view
func NewTable() *Table {
	t := &Table{
		Table: component.NewTable(),
	}
	return t
}

// Init the table view
func (t *Table) Init() {
	t.Table.Init()
	t.SetInputCapture(t.keyboard)
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
