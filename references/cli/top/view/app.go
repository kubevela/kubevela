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
	"log"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/config"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// App application object
type App struct {
	// ui
	*component.App
	// client is the k8s client
	client client.Client
	config config.Config
	// command abstract interface action to a command
	command    *Command
	content    *PageStack
	ctx        context.Context
	cancelFunc context.CancelFunc
}

const (
	delay = time.Second * 10
)

// NewApp return a new app object
func NewApp(c client.Client, restConfig *rest.Config, namespace string) *App {
	conf := config.Config{
		RestConfig: restConfig,
		Theme:      config.LoadThemeConfig(),
	}
	a := &App{
		App:    component.NewApp(conf.Theme),
		client: c,
		config: conf,
		ctx:    context.Background(),
	}
	a.command = NewCommand(a)
	a.content = NewPageStack(a)
	a.ctx = context.WithValue(a.ctx, &model.CtxKeyNamespace, namespace)
	return a
}

// Init the app
func (a *App) Init() {
	a.command.Init()
	a.content.Init()
	a.content.AddListener(a.Menu())
	a.content.AddListener(a.Crumbs())

	a.App.Init()
	a.layout()

	a.bindKeys()
	a.SetInputCapture(a.keyboard)

	a.defaultView(nil)
}

func (a *App) layout() {
	main := tview.NewFlex().SetDirection(tview.FlexRow)
	main.SetBorder(true)
	main.SetBorderColor(a.config.Theme.Border.App.Color())
	main.AddItem(a.buildHeader(), config.HeaderRowNum, 1, false)
	main.AddItem(a.content, 0, 3, true)
	main.AddItem(a.Crumbs(), config.FooterRowNum, 1, false)
	a.Main.AddPage("main", main, true, false)
}

func (a *App) buildHeader() tview.Primitive {
	info := a.InfoBoard()
	info.Init(a.client, a.config.RestConfig)
	header := tview.NewFlex()
	header.SetDirection(tview.FlexColumn)
	header.AddItem(info, 0, 3, false)
	header.AddItem(a.Menu(), 0, 2, false)
	header.AddItem(a.Logo(), config.LogoColumnNum, 3, false)
	return header
}

// Run is the application running entrance
func (a *App) Run() error {
	go func() {
		a.QueueUpdateDraw(func() {
			a.Main.SwitchToPage("main")
		})
	}()
	a.Refresh()
	if err := a.Application.Run(); err != nil {
		return err
	}
	return nil
}

// Refresh will refresh the ui after the delay time
func (a *App) Refresh() {
	ctx := context.Background()
	ctx, a.cancelFunc = context.WithCancel(ctx)
	// system info board component
	board := a.Components()["info"].(*component.InfoBoard)
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("SystemInfo updater canceled!")
				return
			case <-time.After(delay):
				board.UpdateInfo(a.client, a.config.RestConfig)
			}
		}
	}()
}

// inject add a new component to the app's main view to refresh the content of the main view
func (a *App) inject(v model.View) {
	v.Init()
	a.content.PushView(v)
}

func (a *App) bindKeys() {
	a.AddAction(model.KeyActions{
		component.KeyQ:    model.KeyAction{Description: "Back", Action: a.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: a.helpView, Visible: true, Shared: true},
	})
}

func (a *App) keyboard(event *tcell.EventKey) *tcell.EventKey {
	if action, ok := a.HasAction(component.StandardizeKey(event)); ok {
		return action.Action(event)
	}
	return event
}

// defaultView is the first view of running application
func (a *App) defaultView(event *tcell.EventKey) *tcell.EventKey {
	a.command.run(a.ctx, "app")
	return event
}

// helpView to display the view after pressing the Help key(?)
func (a *App) helpView(_ *tcell.EventKey) *tcell.EventKey {
	top := a.content.TopView()
	if top != nil && top.Name() == "Help" {
		a.content.PopView()
		return nil
	}
	helpView := NewHelpView(a)
	a.inject(helpView)
	return nil
}

// Back to return before view corresponding to the ESC key
func (a *App) Back(_ *tcell.EventKey) *tcell.EventKey {
	if !a.content.IsLastView() {
		a.content.PopView()
	}
	return nil
}

// Exist the app
func (a *App) Exist(_ *tcell.EventKey) *tcell.EventKey {
	a.Stop()
	return nil
}

// SwitchTheme switch page to the theme switch page
func (a *App) SwitchTheme(_ *tcell.EventKey) *tcell.EventKey {
	closeFun := func() {
		a.Main.RemovePage("theme")
	}
	selector := component.NewThemeSelector(a.config.Theme, closeFun)
	selector.Init()
	selector.Start()

	a.Main.AddPage("theme", modal(selector.Frame, 45, 30), true, true)
	a.Main.SwitchToPage("theme")
	return nil
}

func modal(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 1, true).
			AddItem(nil, 0, 1, false), width, 1, true).
		AddItem(nil, 0, 1, false)
}
