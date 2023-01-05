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
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// App represent the ui of application
type App struct {
	*tview.Application
	actions    model.KeyActions
	components map[string]tview.Primitive
	Main       *tview.Pages
	style      *config.ThemeConfig
}

// NewApp return the ui of application
func NewApp(themeConfig *config.ThemeConfig) *App {
	a := &App{
		Application: tview.NewApplication(),
		actions:     make(model.KeyActions),
		Main:        tview.NewPages(),
		style:       themeConfig,
	}
	a.components = map[string]tview.Primitive{
		"info":   NewInfo(a.style),
		"menu":   NewMenu(a.style),
		"logo":   NewLogo(a.style),
		"crumbs": NewCrumbs(a.style),
	}
	return a
}

// Init the ui of application init
func (a *App) Init() {
	a.SetRoot(a.Main, true)
}

// HasAction judge whether the key has the corresponding action
func (a *App) HasAction(key tcell.Key) (model.KeyAction, bool) {
	action, ok := a.actions[key]
	return action, ok
}

// AddAction add a new keyAction
func (a *App) AddAction(actions model.KeyActions) {
	a.actions.Add(actions)
}

// DelAction delete a keyAction
func (a *App) DelAction(keys []tcell.Key) {
	a.actions.Delete(keys)
}

// QueueUpdate queues up a ui action.
func (a *App) QueueUpdate(f func()) {
	if a.Application == nil {
		return
	}
	go func() {
		a.Application.QueueUpdate(f)
	}()
}

// QueueUpdateDraw queues up a ui action and redraw the ui.
func (a *App) QueueUpdateDraw(f func()) {
	if a.Application == nil {
		return
	}
	go func() {
		a.Application.QueueUpdateDraw(f)
	}()
}

// Components return the application root components.
func (a *App) Components() map[string]tview.Primitive {
	return a.components
}

// Logo return logo component
func (a *App) Logo() *Logo {
	return a.components["logo"].(*Logo)
}

// Menu return key action menu component
func (a *App) Menu() *Menu {
	return a.components["menu"].(*Menu)
}

// Crumbs return the crumbs component
func (a *App) Crumbs() *Crumbs {
	return a.components["crumbs"].(*Crumbs)
}

// InfoBoard return system info component
func (a *App) InfoBoard() *InfoBoard {
	return a.Components()["info"].(*InfoBoard)
}
