package component

import (
	"github.com/gdamore/tcell/v2"
	"github.com/oam-dev/kubevela/references/cli/top/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMenu(t *testing.T) {
	menu := NewMenu()
	table := NewTable()

	t.Run("stack push", func(t *testing.T) {
		table.actions.Add(model.KeyActions{
			tcell.KeyEnter: model.KeyAction{
				Description: "",
				Action:      nil,
				Visible:     false,
				Shared:      false,
			},
		})
		menu.StackPush(table)
		assert.Equal(t, menu.GetCell(0, 0).Text, " [blue:-:b]<Enter>    [:-:b] ")
	})
	t.Run("stack pop", func(t *testing.T) {
		table.actions.Add(model.KeyActions{
			tcell.KeyEsc: model.KeyAction{
				Description: "",
				Action:      nil,
				Visible:     false,
				Shared:      false,
			},
		})
		menu.StackPop(nil, table)
		assert.Equal(t, menu.GetCell(1, 0).Text, " [blue:-:b]<Esc>      [:-:b] ")
	})
}
