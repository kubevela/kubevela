package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCrumbs(t *testing.T) {
	crumbs := NewCrumbs()
	assert.Equal(t, crumbs.GetItemCount(), 0)
	t.Run("stack push", func(t *testing.T) {
		p := NewPages()
		crumbs.StackPush(p)
		assert.Equal(t, crumbs.GetItemCount(), 2)
	})
	t.Run("stack pop", func(t *testing.T) {
		crumbs.StackPop(nil, nil)
		assert.Equal(t, crumbs.GetItemCount(), 0)
	})
}
