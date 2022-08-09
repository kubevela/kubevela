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
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/stretchr/testify/assert"

	"github.com/oam-dev/kubevela/references/cli/top/model"
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
