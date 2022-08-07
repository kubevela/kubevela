package view

import (
	"github.com/oam-dev/kubevela/references/cli/top/component"
	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// PageStack store views of app's main view, and it's a high level packing of "component.Pages"
type PageStack struct {
	*component.Pages
	app *App
}

// NewPageStack returns a new page stack.
func NewPageStack(app *App) *PageStack {
	ps := &PageStack{
		Pages: component.NewPages(),
		app:   app,
	}
	ps.Init()
	return ps
}

// Init the pageStack
func (ps *PageStack) Init() {
	ps.Stack.AddListener(ps)
}

// StackPop change itself when accept "pop" notify from app's main view
func (ps *PageStack) StackPop(_, _ model.Component) {}

// StackPush change itself when accept "pop" notify from app's main view
func (ps *PageStack) StackPush(component model.Component) {
	ps.app.SetFocus(component)
}
