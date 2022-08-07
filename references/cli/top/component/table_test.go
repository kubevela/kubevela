package component

import (
	"github.com/gdamore/tcell/v2"
	"github.com/oam-dev/kubevela/references/cli/top/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTable(t *testing.T) {
	table := NewTable()
	t.Run("init", func(t *testing.T) {
		table.Init()
		table.Start()
		table.Stop()
	})
	t.Run("name", func(t *testing.T) {
		assert.Equal(t, table.Name(), "table")
	})
	t.Run("action", func(t *testing.T) {
		assert.Equal(t, len(table.Actions()), 0)
	})
	t.Run("action and hint", func(t *testing.T) {
		table.actions.Add(model.KeyActions{
			tcell.KeyEnter: model.KeyAction{
				Description: "",
				Action:      nil,
				Visible:     false,
				Shared:      false,
			},
		})
		assert.Equal(t, len(table.Actions()), 1)
		assert.Equal(t, len(table.Hint()), 1)
		assert.Equal(t, table.Hint()[0], model.MenuHint{
			Key:         "Enter",
			Description: "",
		})
	})
}
