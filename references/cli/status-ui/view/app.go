package view

import (
	"context"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/oam-dev/kubevela/references/cli/status-ui/config"
	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type App struct {
	*ui.App

	client  client.Client
	command *Command
	config  config.Config
	Content *PageStack
}

func NewApp(c client.Client, restConfig *rest.Config) *App {
	a := &App{
		App:    ui.NewApp(),
		client: c,
		config: config.Config{
			RestConfig: restConfig,
		},
	}
	a.command = NewCommand(a)
	a.Content = NewPageStack(a)

	return a
}

func (a *App) Init() {
	a.command.Init()
	a.Content.Init()
	a.Content.AddListener(a.Menu())
	a.Content.AddListener(a.Crumbs())

	a.App.Init()
	a.layout()

	a.bindKeys()
	a.SetInputCapture(a.keyboard)

	a.defaultView(nil)
}

func (a *App) layout() {
	main := tview.NewFlex().SetDirection(tview.FlexRow)
	main.SetBorder(true)
	main.SetBorderAttributes(tcell.RuneBoard)
	main.SetBorderPadding(0, 0, 1, 1)
	main.AddItem(a.buildHeader(), ui.HEADER_ROW_NUM, 1, false)
	main.AddItem(a.Content, 0, 3, true)
	main.AddItem(a.Crumbs(), ui.FOOTER_ROW_NUM, 1, false)
	a.Main.AddPage("main", main, true, false)
}

func (a *App) buildHeader() tview.Primitive {
	header := tview.NewFlex()
	header.SetDirection(tview.FlexColumn)
	info := a.InfoBoard()
	info.Init(a.config.RestConfig)
	header.AddItem(info, 30, 1, false)
	header.AddItem(a.Menu(), 0, 2, false)
	header.AddItem(a.Logo(), 45, 1, false)
	return header
}

func (a *App) Run() {
	go func() {
		a.QueueUpdateDraw(func() {
			a.Main.SwitchToPage("main")
		})
	}()
	err := a.Application.Run()
	if err != nil {
		return
	}
}

func (a *App) bindKeys() {
	a.AddAction(model.KeyActions{
		tcell.KeyESC: model.KeyAction{Description: "Back", Action: a.Back, Visible: true, Shared: true},
		ui.KeyHelp:   model.KeyAction{Description: "Help", Action: a.helpView, Visible: true, Shared: true},
	})
}

func (a *App) keyboard(event *tcell.EventKey) *tcell.EventKey {
	if action, ok := a.HasAction(ui.StandardizeKey(event)); ok {
		return action.Action(event)
	}
	return event
}

func (a *App) inject(c model.Component) {
	c.Init()
	a.Content.PushComponent(c)
}

func (a *App) defaultView(event *tcell.EventKey) *tcell.EventKey {
	ctx := context.Background()
	a.command.run(ctx, "app")
	return event
}

func (a *App) helpView(event *tcell.EventKey) *tcell.EventKey {
	top := a.Content.TopComponent()
	if top != nil && top.Name() == "help" {
		a.Content.PopComponent()
		return nil
	}

	helpView := NewHelpView(a)
	a.inject(helpView)
	return nil
}

func (a *App) Back(evt *tcell.EventKey) *tcell.EventKey {
	if !a.Content.IsLastComponent() {
		a.Content.PopComponent()
	}

	return nil
}
