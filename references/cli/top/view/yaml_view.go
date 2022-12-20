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
	"regexp"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// YamlView is the resource yaml view, this view display info of yaml text of the resource
type YamlView struct {
	*tview.TextView
	app     *App
	actions model.KeyActions
	ctx     context.Context
}

var (
	keyValRX         = regexp.MustCompile(`\A(\s*)([\w|\-|\.|\/|\s]+):\s(.+)\z`)
	keyRX            = regexp.MustCompile(`\A(\s*)([\w|\-|\.|\/|\s]+):\s*\z`)
	yamlViewInstance = new(YamlView)
)

const (
	yamlFullFmt  = "%s[key::b]%s[colon::-]: [val::]%s"
	yamlKeyFmt   = "%s[key::b]%s[colon::-]:"
	yamlValueFmt = "[val::]%s"
)

// NewYamlView return a new yaml view
func NewYamlView(ctx context.Context, app *App) model.View {
	yamlViewInstance.ctx = ctx
	if yamlViewInstance.TextView == nil {
		yamlViewInstance.TextView = tview.NewTextView()
		yamlViewInstance.actions = make(model.KeyActions)
		yamlViewInstance.app = app
	}
	return yamlViewInstance
}

// Init the yaml view
func (v *YamlView) Init() {
	title := fmt.Sprintf("[ %s ]", v.Name())
	v.SetDynamicColors(true)
	v.SetRegions(true)
	v.SetBorder(true)
	v.SetBorderAttributes(tcell.AttrItalic)
	v.SetTitle(title).SetTitleColor(v.app.config.Theme.Table.Title.Color())
	v.bindKeys()
	v.SetInputCapture(v.keyboard)
}

// Start the yaml view
func (v *YamlView) Start() {
	v.Clear()
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
		yaml = v.HighlightText(yaml)
		v.SetText(yaml)
	} else {
		v.SetText("can't load the Yaml text of the resource!")
	}
}

// Stop the yaml view
func (v *YamlView) Stop() {
	v.Clear()
}

// Name return the name of yaml view
func (v *YamlView) Name() string {
	return "Yaml"
}

// Hint return the menu hints of yaml view
func (v *YamlView) Hint() []model.MenuHint {
	return v.actions.Hint()
}

// HighlightText highlight the key, colon, value text of the yaml text
func (v *YamlView) HighlightText(yaml string) string {
	lines := strings.Split(tview.Escape(yaml), "\n")

	fullFmt := strings.Replace(yamlFullFmt, "[key", "["+v.app.config.Theme.Yaml.Key.String(), 1)
	fullFmt = strings.Replace(fullFmt, "[colon", "["+v.app.config.Theme.Yaml.Colon.String(), 1)
	fullFmt = strings.Replace(fullFmt, "[val", "["+v.app.config.Theme.Yaml.Value.String(), 1)

	keyFmt := strings.Replace(yamlKeyFmt, "[key", "["+v.app.config.Theme.Yaml.Key.String(), 1)
	keyFmt = strings.Replace(keyFmt, "[colon", "["+v.app.config.Theme.Yaml.Colon.String(), 1)

	valFmt := strings.Replace(yamlValueFmt, "[val", "["+v.app.config.Theme.Yaml.Value.String(), 1)

	buff := make([]string, 0, len(lines))
	for _, l := range lines {
		res := keyValRX.FindStringSubmatch(l)
		if len(res) == 4 {
			buff = append(buff, fmt.Sprintf(fullFmt, res[1], res[2], res[3]))
			continue
		}
		res = keyRX.FindStringSubmatch(l)
		if len(res) == 3 {
			buff = append(buff, fmt.Sprintf(keyFmt, res[1], res[2]))
			continue
		}

		buff = append(buff, fmt.Sprintf(valFmt, l))
	}

	return strings.Join(buff, "\n")
}

func (v *YamlView) keyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	if key == tcell.KeyUp || key == tcell.KeyDown {
		return event
	}
	if a, ok := v.actions[component.StandardizeKey(event)]; ok {
		return a.Action(event)
	}
	return event
}

func (v *YamlView) bindKeys() {
	v.actions.Delete([]tcell.Key{tcell.KeyEnter})
	v.actions.Add(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: v.app.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: v.app.helpView, Visible: true, Shared: true},
	})
}
