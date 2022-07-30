package view

import (
	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
	"github.com/oam-dev/kubevela/references/cli/status-ui/ui"
)

type PageStack struct {
	*ui.Pages
	app *App
}

// NewPageStack returns a new page stack.
func NewPageStack(app *App) *PageStack {
	ps := &PageStack{
		Pages: ui.NewPages(),
		app:   app,
	}
	ps.Stack.AddListener(ps)
	return ps
}

func (p *PageStack) Init() {
}

func (p *PageStack) StackPop(old, new model.Component) {

}

func (p *PageStack) StackPush(component model.Component) {
	p.app.SetFocus(component)
}

func (p *PageStack) LastPage() bool {
	return p.Stack.Empty()
}
