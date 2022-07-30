package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/config"
	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type App struct {
	*tview.Application
	actions model.KeyActions

	Components map[string]tview.Primitive
	Main       *Pages
	Style      *config.Style
}

func NewApp() *App {
	a := App{
		Application: tview.NewApplication(),
		actions:     make(model.KeyActions),
		Main:        NewPages(),
	}
	a.Components = map[string]tview.Primitive{
		"clusterInfo": NewClusterInfo(a.Style),
		"menu":        NewMenu(a.Style),
		"logo":        NewLogo(a.Style),
		"crumbs":      NewCrumbs(a.Style),
	}
	return &a
}

func (a *App) Init() {
	a.bindKeys()
	a.SetRoot(a.Main, true)
}

func (a *App) bindKeys() {

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

func (a *App) ClusterInfo() *ClusterInfo {
	return a.Components["clusterInfo"].(*ClusterInfo)
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
