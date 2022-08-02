package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type App struct {
	*tview.Application
	actions    model.KeyActions
	Components map[string]tview.Primitive
	Main       *Pages
}

func NewApp() *App {
	a := &App{
		Application: tview.NewApplication(),
		actions:     make(model.KeyActions),
		Main:        NewPages(),
	}
	a.Components = map[string]tview.Primitive{
		"info":   NewInfo(),
		"menu":   NewMenu(),
		"logo":   NewLogo(),
		"crumbs": NewCrumbs(),
	}
	return a
}

func (a *App) Init() {
	a.SetRoot(a.Main, true)
}

func (a *App) HasAction(key tcell.Key) (model.KeyAction, bool) {
	action, ok := a.actions[key]
	return action, ok
}

func (a *App) AddAction(actions model.KeyActions) {
	a.actions.Add(actions)
}

func (a *App) DelAction(keys []tcell.Key) {
	a.actions.Delete(keys)
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

func (a *App) InfoBoard() *InfoBoard {
	return a.Components["info"].(*InfoBoard)
}

func (a *App) Logo() *Logo {
	return a.Components["logo"].(*Logo)
}

func (a *App) Menu() *Menu {
	return a.Components["menu"].(*Menu)
}

func (a *App) Crumbs() *Crumbs {
	return a.Components["crumbs"].(*Crumbs)
}
