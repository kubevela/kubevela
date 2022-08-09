/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
