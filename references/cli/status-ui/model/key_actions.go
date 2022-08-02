package model

import (
	"sort"

	"github.com/gdamore/tcell/v2"
)

type KeyAction struct {
	Description string
	Action      func(*tcell.EventKey) *tcell.EventKey
	Visible     bool
	Shared      bool
}
type KeyActions map[tcell.Key]KeyAction

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

func (ka KeyActions) Add(actions KeyActions) {
	for k, v := range actions {
		ka[k] = v
	}
}

func (ka KeyActions) Set(actions KeyActions) {
	for k, v := range actions {
		ka[k] = v
	}
}

func (ka KeyActions) Delete(kk []tcell.Key) {
	for _, k := range kk {
		delete(ka, k)
	}
}

func (ka KeyActions) Clear() {
	for k := range ka {
		delete(ka, k)
	}
}
