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
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// HelpView is the view which display help tips about how to use app
type HelpView struct {
	*tview.TextView
	app     *App
	actions model.KeyActions
}

var (
	helpViewInstance = new(HelpView)
)

// NewHelpView return a new help view
func NewHelpView(app *App) model.View {
	if helpViewInstance.TextView == nil {
		helpViewInstance.TextView = tview.NewTextView()
		helpViewInstance.app = app
		helpViewInstance.actions = make(model.KeyActions)
	}
	return helpViewInstance
}

// Init help view init
func (v *HelpView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetDynamicColors(true)
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetBorderPadding(1, 1, 3, 3)
	v.bindKeys()
}

func (v *HelpView) Start() {
	v.SetText(`
[blue:]vela top[white:] is a UI based CLI tool provided in KubeVela. By using it, you can obtain the overview information of the platform and diagnose the resource status of the application.

At present, the tool has provided the following feature:

[blue:]*[white:] Platform information overview
[blue:]*[white:] Display of resource status information at Application, Managed Resource and Pod levels
[blue:]*[white:] Application Resource Topology
[blue:]*[white:] Resource YAML text display

This information panel component in UI header will display the performance information of the KubeVela system.

Resource tables are in the UI body, three levels resource are displayed here. You can use the ENTER key to enter the next resource level or the Q key to return to the previous level.

The crumbs component in the footer indicates the current resource level.
`)
}

func (v *HelpView) Stop() {}

// Name return help view name
func (v *HelpView) Name() string {
	return "Help"
}

// Hint return the menu hints of yaml view
func (v *HelpView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

func (v *HelpView) bindKeys() {
	v.actions.Add(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
	})
}
