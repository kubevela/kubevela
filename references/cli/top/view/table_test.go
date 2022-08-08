package view

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

func TestTable(t *testing.T) {
	table := NewTable()

	t.Run("init", func(t *testing.T) {
		table.Init()
		assert.Equal(t, table.Table.GetBorderAttributes(), tcell.AttrItalic)
	})

	t.Run("keyboard", func(t *testing.T) {
		evt1 := tcell.NewEventKey(tcell.KeyUp, '/', 0)
		assert.Equal(t, table.keyboard(evt1), evt1)
		evt2 := tcell.NewEventKey(tcell.KeyEsc, '/', 0)
		assert.Equal(t, table.keyboard(evt2), evt2)
	})
}
