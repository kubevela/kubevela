package model

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"
)

func TestKeyActions_Add(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Add(KeyActions{tcell.KeyTAB: KeyAction{}})
	assert.Equal(t, len(actions), 2)
}

func TestKeyActions_Clear(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Clear()
	assert.Equal(t, len(actions), 0)
}

func TestKeyActions_Delete(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Delete([]tcell.Key{tcell.KeyEnter})
	assert.Equal(t, len(actions), 0)
}

func TestKeyActions_Hint(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	hints := actions.Hint()
	assert.Equal(t, len(hints), 1)
	assert.Equal(t, hints[0], MenuHint{
		Key:         "Enter",
		Description: "",
	})
}

func TestKeyActions_Set(t *testing.T) {
	actions := KeyActions{tcell.KeyEnter: KeyAction{}}
	actions.Set(KeyActions{tcell.KeyHelp: KeyAction{
		Description: "",
		Action:      nil,
		Visible:     false,
		Shared:      false,
	}})
	assert.Equal(t, len(actions), 2)
}
