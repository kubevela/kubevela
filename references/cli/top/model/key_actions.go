package model

import (
	"sort"

	"github.com/gdamore/tcell/v2"
)

// KeyAction is key action struct
type KeyAction struct {
	Description string
	Action      func(*tcell.EventKey) *tcell.EventKey
	Visible     bool
	Shared      bool
}

// KeyActions is a map from key to action
type KeyActions map[tcell.Key]KeyAction

// Hint convert key action map to menu hints
func (ka KeyActions) Hint() []MenuHint {
	tmp := make([]int, 0)
	for k := range ka {
		tmp = append(tmp, int(k))
	}
	sort.Ints(tmp)
	hints := make([]MenuHint, 0)

	for _, key := range tmp {
		if name, ok := tcell.KeyNames[tcell.Key(key)]; ok {
			hints = append(hints,
				MenuHint{
					Key:         name,
					Description: ka[tcell.Key(key)].Description,
				},
			)
		}
	}
	return hints
}

// Add a key action to key action map
func (ka KeyActions) Add(actions KeyActions) {
	for k, v := range actions {
		ka[k] = v
	}
}

// Set a key action to key action map
func (ka KeyActions) Set(actions KeyActions) {
	for k, v := range actions {
		ka[k] = v
	}
}

// Delete aim key from key action map
func (ka KeyActions) Delete(kk []tcell.Key) {
	for _, k := range kk {
		delete(ka, k)
	}
}

// Clear key action map clear up
func (ka KeyActions) Clear() {
	for k := range ka {
		delete(ka, k)
	}
}
