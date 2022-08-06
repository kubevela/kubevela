package view

import (
	"context"

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
	*component.App
	client  client.Client
	config  config.Config
	command *Command
	content *PageStack
}

// NewApp return a new app object
func NewApp(c client.Client, restConfig *rest.Config) *App {
	a := &App{
		App:    component.NewApp(),
		client: c,
		config: config.Config{
			RestConfig: restConfig,
		},
	}
	a.command = NewCommand(a)
	a.content = NewPageStack(a)
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
	main.SetBorderPadding(0, 0, 1, 1)
	main.AddItem(a.buildHeader(), config.HeaderRowNum, 1, false)
	main.AddItem(a.content, 0, 3, true)
	main.AddItem(a.Crumbs(), config.FooterRowNum, 1, false)
	a.Main.AddPage("main", main, true, false)
}

func (a *App) buildHeader() tview.Primitive {
	header := tview.NewFlex()
	header.SetDirection(tview.FlexColumn)
	info := a.InfoBoard()
	info.Init(a.config.RestConfig)
	header.AddItem(info, config.InfoColumnNum, 1, false)
	header.AddItem(a.Menu(), 0, 2, false)
	header.AddItem(a.Logo(), config.LogoColumnNum, 1, false)
	return header
}

// Run is the application running entrance
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
		tcell.KeyESC:      model.KeyAction{Description: "Back", Action: a.Back, Visible: true, Shared: true},
		component.KeyHelp: model.KeyAction{Description: "Help", Action: a.helpView, Visible: true, Shared: true},
	})
}

func (a *App) keyboard(event *tcell.EventKey) *tcell.EventKey {
	if action, ok := a.HasAction(component.StandardizeKey(event)); ok {
		return action.Action(event)
	}
	return event
}

// inject add a new component to the app's main view to refresh the content of the main view
func (a *App) inject(c model.Component) {
	c.Init()
	a.content.PushComponent(c)
}

// defaultView is the first view of running application
func (a *App) defaultView(event *tcell.EventKey) *tcell.EventKey {
	ctx := context.Background()
	ctx = context.WithValue(ctx, model.CtxKeyNamespace, "")
	a.command.run(ctx, "app")
	return event
}

// helpView to display the view after pressing the Help key(?)
func (a *App) helpView(_ *tcell.EventKey) *tcell.EventKey {
	top := a.content.TopComponent()
	if top != nil && top.Name() == "help" {
		a.content.PopComponent()
		return nil
	}
	helpView := NewHelpView(a)
	a.inject(helpView)
	return nil
}

// Back to return before view corresponding to the ESC key
func (a *App) Back(_ *tcell.EventKey) *tcell.EventKey {
	if !a.content.IsLastComponent() {
		a.content.PopComponent()
	}
	return nil
}
