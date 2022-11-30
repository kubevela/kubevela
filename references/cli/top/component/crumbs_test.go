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
	"github.com/rivo/tview"
	"testing"

	"github.com/oam-dev/kubevela/references/cli/top/config"

	"github.com/stretchr/testify/assert"
)

func TestCrumbs(t *testing.T) {
	crumbs := NewCrumbs(&config.ThemeConfig{
		Crumbs: struct {
			Foreground config.Color
			Background config.Color
		}{
			Foreground: "white",
			Background: "red",
		},
	})
	assert.Equal(t, crumbs.GetItemCount(), 0)
	t.Run("stack push", func(t *testing.T) {
		p := NewPages()
		crumbs.StackPush(nil, p)
		assert.Equal(t, crumbs.GetItemCount(), 2)
		textView := crumbs.GetItem(0).(*tview.TextView)
		assert.Equal(t, textView.GetBackgroundColor(), crumbs.style.Crumbs.Background.Color())
	})
	t.Run("stack pop", func(t *testing.T) {
		crumbs.StackPop(nil, nil)
		assert.Equal(t, crumbs.GetItemCount(), 0)
	})
}
