package ui

import (
	"fmt"
	"strconv"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/config"
	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Menu struct {
	*tview.Table
	Style *config.Style
}

func NewMenu(style *config.Style) *Menu {
	m := &Menu{
		Table: tview.NewTable(),
		Style: style,
	}
	m.init()
	return m
}

func (m *Menu) init() {

}

func (m *Menu) StackPop(old, new model.Component) {
	m.Clear()
	m.UpdateMenu(new.Hint())
}

func (m *Menu) StackPush(component model.Component) {
	m.UpdateMenu(component.Hint())
}

func (m *Menu) UpdateMenu(hints []model.MenuHint) {
	m.Clear()

	table := make([][]model.MenuHint, MENU_ROW_NUM)
	columCount := (len(hints) / MENU_ROW_NUM) + 1

	for row := 0; row < MENU_ROW_NUM; row++ {
		table[row] = make([]model.MenuHint, columCount)
	}

	t := m.buildMenuTable(hints, table, columCount)

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

func (m *Menu) buildMenuTable(hints []model.MenuHint, table [][]model.MenuHint, columCount int) [][]string {
	var row, col int

	for _, h := range hints {
		table[row][col] = h
		row++
		if row >= MENU_ROW_NUM {
			row, col = 0, col+1
		}
	}

	out := make([][]string, len(table))
	for r := range out {
		out[r] = make([]string, len(table[r]))
	}
	m.layout(table, out)
	return out
}

func (m *Menu) layout(table [][]model.MenuHint, out [][]string) {
	for r := range table {
		for c := range table[r] {
			out[r][c] = formatPlainMenu(table[r][c])
		}
	}
}

func menuFormat(hint model.MenuHint) string {
	if len(hint.Key) == 0 && len(hint.Description) == 0 {
		return ""
	}
	return fmt.Sprintf("<%s>", hint.Key)
}

func formatPlainMenu(h model.MenuHint) string {
	menuFmt := " [key:-:b]%-" + strconv.Itoa(10) + "s [fg:-:d]%s "
	return fmt.Sprintf(menuFmt, menuFormat(h), h.Description)
}
