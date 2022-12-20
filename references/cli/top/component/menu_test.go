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

package component

import (
	"fmt"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

func TestMenu(t *testing.T) {
	menu := NewMenu(&themeConfig)
	table := NewTable(&themeConfig)

	t.Run("stack push", func(t *testing.T) {
		table.actions.Add(model.KeyActions{
			tcell.KeyEnter: model.KeyAction{
				Description: "",
				Action:      nil,
				Visible:     false,
				Shared:      false,
			},
		})
		menu.StackPush(nil, table)
		menuHint := fmt.Sprintf("[%s:-:b]%-8s [%s:-:b]%s", menu.style.Menu.Key.String(), "<Enter>", menu.style.Menu.Description.String(), "")
		assert.Equal(t, menu.GetCell(0, 0).Text, menuHint)
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
		menuHint := fmt.Sprintf("[%s:-:b]%-8s [%s:-:b]%s", menu.style.Menu.Key.String(), "<Esc>", menu.style.Menu.Description.String(), "")
		assert.Equal(t, menu.GetCell(1, 0).Text, menuHint)
	})
}
