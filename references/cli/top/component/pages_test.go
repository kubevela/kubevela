package component

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPages(t *testing.T) {
	pages := NewPages()
	table := NewTable()

	t.Run("init", func(t *testing.T) {
		pages.Init()
		pages.Start()
		pages.Stop()
		assert.Equal(t, pages.Stack.Empty(), true)
	})
	t.Run("name", func(t *testing.T) {
		assert.Equal(t, pages.Name(), "Pages")
	})
	t.Run("hint", func(t *testing.T) {
		assert.Equal(t, len(pages.Hint()), 0)
	})

	t.Run("component id", func(t *testing.T) {
		assert.Contains(t, componentID(table), "table")
	})
	t.Run("stack push", func(t *testing.T) {
		pages.StackPush(table)
		assert.Equal(t, pages.HasPage(componentID(table)), true)
	})
	t.Run("stack pop", func(t *testing.T) {
		pages.StackPop(table, nil)
		assert.Equal(t, pages.HasPage(componentID(table)), false)
	})
}
