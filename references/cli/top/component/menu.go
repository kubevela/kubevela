package component

import (
	"fmt"
	"math"
	"strconv"

	"github.com/oam-dev/kubevela/references/cli/top/config"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Menu is menu component which display key actions of app's main view
type Menu struct {
	*tview.Table
}

// NewMenu return a new menu instance
func NewMenu() *Menu {
	m := &Menu{
		Table: tview.NewTable(),
	}
	return m
}

// StackPop change itself when accept "pop" notify from app's main view
func (m *Menu) StackPop(old, new model.Component) {
	m.UpdateMenu(new.Hint())
}

// StackPush change itself when accept "push" notify from app's main view
func (m *Menu) StackPush(component model.Component) {
	m.UpdateMenu(component.Hint())
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
			out[r][c] = formatPlainMenu(table[r][c])
		}
	}
	return out
}

func formatPlainMenu(h model.MenuHint) string {
	menuFmt := " [blue:-:b]%-" + strconv.Itoa(10) + "s [:-:b]%s "
	return fmt.Sprintf(menuFmt, menuFormat(h), h.Description)
}

func menuFormat(hint model.MenuHint) string {
	if len(hint.Key) == 0 && len(hint.Description) == 0 {
		return ""
	}
	return fmt.Sprintf("<%s>", hint.Key)
}
