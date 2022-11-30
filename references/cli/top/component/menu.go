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
	"fmt"
	"math"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Menu is menu component which display key actions of app's main view
type Menu struct {
	*tview.Table
	style *config.ThemeConfig
}

// NewMenu return a new menu instance
func NewMenu(config *config.ThemeConfig) *Menu {
	m := &Menu{
		Table: tview.NewTable(),
		style: config,
	}
	return m
}

// StackPop change itself when accept "pop" notify from app's main view
func (m *Menu) StackPop(_, new model.View) {
	if new == nil {
		m.UpdateMenu([]model.MenuHint{})
	} else {
		m.UpdateMenu(new.Hint())
	}

}

// StackPush change itself when accept "push" notify from app's main view
func (m *Menu) StackPush(_, new model.View) {
	m.UpdateMenu(new.Hint())
}

// UpdateMenu update menu component
func (m *Menu) UpdateMenu(hints []model.MenuHint) {
	m.Clear()
	// convert one-dimensional hints to two-dimensional hints
	tableCellStrHint := make([][]model.MenuHint, config.MenuRowNum)
	columCount := int(math.Ceil(float64(len(hints)) / config.MenuRowNum))
	for row := 0; row < config.MenuRowNum; row++ {
		tableCellStrHint[row] = make([]model.MenuHint, columCount)
	}

	// convert two-dimensional hints to two-dimensional string
	tableCellStr := m.buildMenuTable(hints, tableCellStrHint)
	for row := 0; row < len(tableCellStr); row++ {
		for col := 0; col < len(tableCellStr[row]); col++ {
			c := tview.NewTableCell(tableCellStr[row][col])
			if len(tableCellStr[row][col]) == 0 {
				c = tview.NewTableCell("")
			}
			m.SetCell(row, col, c)
		}
	}
}

func (m *Menu) buildMenuTable(hints []model.MenuHint, table [][]model.MenuHint) [][]string {
	var row, col int
	for _, h := range hints {
		table[row][col] = h
		row++
		if row == config.MenuRowNum {
			row, col = 0, col+1
		}
	}
	out := make([][]string, len(table))
	for r := range out {
		out[r] = make([]string, len(table[r]))
	}
	for r := range table {
		for c := range table[r] {
			out[r][c] = m.formatPlainMenu(table[r][c])
		}
	}
	return out
}

func (m *Menu) formatPlainMenu(h model.MenuHint) string {
	return fmt.Sprintf("[%s:-:b]%-8s [%s:-:b]%s", m.style.Menu.Key, menuFormat(h), m.style.Menu.Description, h.Description)
}

func menuFormat(hint model.MenuHint) string {
	if len(hint.Key) == 0 && len(hint.Description) == 0 {
		return ""
	}
	return fmt.Sprintf("<%s>", hint.Key)
}
