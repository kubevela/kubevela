package component

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// App represent the ui of application
type App struct {
	*tview.Application
	actions    model.KeyActions
	Components map[string]tview.Primitive
	Main       *Pages
}

// NewApp return the ui of application
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

// QueueUpdateDraw queues up a ui action and redraw the ui.
func (a *App) QueueUpdateDraw(f func()) {
	if a.Application == nil {
		return
	}
	go func() {
		a.Application.QueueUpdateDraw(f)
	}()
}

// InfoBoard return system info component
func (a *App) InfoBoard() *InfoBoard {
	return a.Components["info"].(*InfoBoard)
}

// Logo return logo component
func (a *App) Logo() *Logo {
	return a.Components["logo"].(*Logo)
}

// Menu return key action menu component
func (a *App) Menu() *Menu {
	return a.Components["menu"].(*Menu)
}

// Crumbs return the crumbs component
func (a *App) Crumbs() *Crumbs {
	return a.Components["crumbs"].(*Crumbs)
}
