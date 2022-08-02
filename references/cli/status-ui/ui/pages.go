package ui

import (
	"fmt"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Pages struct {
	*tview.Pages
	*model.Stack
}

// NewPages return a new view.
func NewPages() *Pages {
	p := &Pages{
		Pages: tview.NewPages(),
		Stack: model.NewStack(),
	}
	p.Stack.AddListener(p)
	return p
}

func (p *Pages) StackPop(old, new model.Component) {
	p.delete(old)
}

func (p *Pages) StackPush(component model.Component) {
	p.addAndShow(component)
}

// AddAndShow adds a new page and bring it to front.
func (p *Pages) addAndShow(c model.Component) {
	p.add(c)
	p.Show(c)
}

// Add adds a new page.
func (p *Pages) add(c model.Component) {
	p.AddPage(componentID(c), c, true, true)
}

func (p *Pages) delete(c model.Component) {
	p.RemovePage(componentID(c))
}

func (p *Pages) Show(c model.Component) {
	p.SwitchToPage(componentID(c))
}

func componentID(c model.Component) string {
	if c.Name() == "" {
		panic("Component has no name")
	}
	return fmt.Sprintf("%s-%p", c.Name(), c)
}
