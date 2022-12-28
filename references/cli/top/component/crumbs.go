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

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Crumbs component lay on footer of app and indicate resource level
type Crumbs struct {
	*tview.Flex
	style *config.ThemeConfig
}

// NewCrumbs return a new crumbs component
func NewCrumbs(config *config.ThemeConfig) *Crumbs {
	c := &Crumbs{
		Flex:  tview.NewFlex(),
		style: config,
	}
	return c
}

// StackPop change itself when accept "pop" notify from app's main view
func (c *Crumbs) StackPop(_, _ model.View) {
	num := c.GetItemCount()
	if num >= 2 {
		c.RemoveItem(c.GetItem(num - 1))
		c.RemoveItem(c.GetItem(num - 2))
	}
}

// StackPush change itself when accept "push" notify from app's main view
func (c *Crumbs) StackPush(_, new model.View) {
	name := new.Name()
	t := tview.NewTextView()
	t.SetTextColor(c.style.Crumbs.Foreground.Color())
	t.SetBackgroundColor(c.style.Crumbs.Background.Color())
	t.SetTextAlign(tview.AlignCenter)

	t.SetText(name)
	c.AddItem(t, len(name)+2, 0, false)
	c.AddItem(tview.NewTextView(), 1, 0, false)
}
