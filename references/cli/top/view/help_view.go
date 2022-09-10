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

package view

import (
	"fmt"

	"github.com/gdamore/tcell/v2"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// HelpView is the view which display help tips about how to use app
type HelpView struct {
	*component.Table
	app *App
}

// NewHelpView return a new help view
func NewHelpView(app *App) *HelpView {
	v := &HelpView{
		Table: component.NewTable(),
		app:   app,
	}
	return v
}

// Init help view init
func (v *HelpView) Init() {
	v.Table.Init()
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	v.bindKeys()
}

// Name return help view name
func (v *HelpView) Name() string {
	return "Help"
}

func (v *HelpView) bindKeys() {
	v.Actions().Add(model.KeyActions{
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
	})
}
