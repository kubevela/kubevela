package ui

import (
	"fmt"
	"math"
	"strconv"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Menu struct {
	*tview.Table
}

func NewMenu() *Menu {
	m := &Menu{
		Table: tview.NewTable(),
	}
	m.init()
	return m
}

func (m *Menu) init() {
}

func (m *Menu) StackPop(old, new model.Component) {
	m.UpdateMenu(new.Hint())
}

func (m *Menu) StackPush(component model.Component) {
	m.UpdateMenu(component.Hint())
}

func (m *Menu) UpdateMenu(hints []model.MenuHint) {
	m.Clear()
	table := make([][]model.MenuHint, MENU_ROW_NUM)
	columCount := int(math.Ceil(float64(len(hints)) / MENU_ROW_NUM))
	for row := 0; row < MENU_ROW_NUM; row++ {
		table[row] = make([]model.MenuHint, columCount)
	}

	t := m.buildMenuTable(hints, table)

	for row := 0; row < len(t); row++ {
		for col := 0; col < len(t[row]); col++ {
			c := tview.NewTableCell(t[row][col])
			if len(t[row][col]) == 0 {
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
		if row == MENU_ROW_NUM {
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
