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

func TestTable(t *testing.T) {
	table := NewTable(&themeConfig)
	t.Run("init", func(t *testing.T) {
		table.Init()
		assert.Equal(t, table.GetBorderColor(), themeConfig.Border.Table.Color())
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
