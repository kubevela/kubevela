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

func TestApp(t *testing.T) {
	app := NewApp()
	assert.Equal(t, len(app.actions), 0)
	assert.Equal(t, len(app.Components()), 4)
	t.Run("app init", func(t *testing.T) {
		app.Init()
		app.QueueUpdateDraw(func() {})
	})
	t.Run("add action", func(t *testing.T) {
		app.AddAction(model.KeyActions{
			tcell.KeyEnter: model.KeyAction{
				Description: "",
				Action:      nil,
				Visible:     false,
				Shared:      false,
			},
		})
		assert.Equal(t, len(app.actions), 1)
	})
	t.Run("delete action", func(t *testing.T) {
		app.DelAction([]tcell.Key{tcell.KeyEnter})
		assert.Equal(t, len(app.actions), 0)
	})
	t.Run("menu", func(t *testing.T) {
		assert.NotEmpty(t, app.Menu())
	})
	t.Run("crumbs", func(t *testing.T) {
		assert.NotEmpty(t, app.Crumbs())
	})
	t.Run("logo", func(t *testing.T) {
		assert.NotEmpty(t, app.Logo())
	})
	t.Run("info board", func(t *testing.T) {
		assert.NotEmpty(t, app.InfoBoard())
	})
}
