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
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// YamlView is the resource yaml view, this view display info of yaml text of the resource
type YamlView struct {
	*tview.TextView
	app     *App
	actions model.KeyActions
	ctx     context.Context
}

// NewYamlView return  a new yaml view
func NewYamlView(ctx context.Context, app *App) model.Component {
	v := &YamlView{
		TextView: tview.NewTextView(),
		actions:  map[tcell.Key]model.KeyAction{},
		app:      app,
		ctx:      ctx,
	}
	return v
}

// Init the yaml view
func (v *YamlView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetTitle(title).SetTitleColor(config.ResourceTableTitleColor)
	v.bindKeys()
	v.SetInputCapture(v.keyboard)
	v.Start()
}

// Start the yaml view
func (v *YamlView) Start() {
	gvr, ok := v.ctx.Value(&model.CtxKeyGVR).(*model.GVR)
	if ok {
		obj, err := model.GetResourceObject(v.app.client, gvr)
		if err != nil {
			v.SetText(fmt.Sprintf("can't load the Yaml text of the resource!, because  %s", err))
			return
		}
		yaml, err := model.ToYaml(obj)
		if err != nil {
			v.SetText(fmt.Sprintf("can't load the Yaml text of the resource!, because  %s", err))
			return
		}
		v.SetText(yaml)
	} else {
		v.SetText("can't load the Yaml text of the resource!")
	}
}

// Stop the yaml view
func (v *YamlView) Stop() {
}

// Name return the name of yaml view
func (v *YamlView) Name() string {
	return "Yaml"
}

// Hint return the menu hints of yaml view
func (v *YamlView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

func (v *YamlView) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}
	if a, ok := v.actions[tcell.Key(event.Rune())]; ok {
		return a.Action(event)
	}
	return event
}

func (v *YamlView) bindKeys() {
	v.actions.Delete([]tcell.Key{tcell.KeyEnter})
	v.actions.Add(model.KeyActions{
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
