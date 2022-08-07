package component

import (
	"fmt"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Pages is the app's main content view component
type Pages struct {
	*tview.Pages
	*model.Stack
}

// NewPages return a page component
func NewPages() *Pages {
	p := &Pages{
		Pages: tview.NewPages(),
		Stack: model.NewStack(),
	}
	p.Stack.AddListener(p)
	return p
}

// Init table component
func (p *Pages) Init() {}

// Name return pages' name
func (p *Pages) Name() string {
	return "Pages"
}

// Start table component
func (p *Pages) Start() {}

// Stop table component
func (p *Pages) Stop() {}

// Hint return key action menu hints of the component
func (p *Pages) Hint() []model.MenuHint {
	return []model.MenuHint{}
}

// StackPop change itself when accept "pop" notify from app's main view
func (p *Pages) StackPop(old, _ model.Component) {
	p.delete(old)
}

// StackPush change itself when accept "push" notify from app's main view
func (p *Pages) StackPush(component model.Component) {
	p.addAndShow(component)
}

// AddAndShow adds a new page and bring it to front.
func (p *Pages) addAndShow(c model.Component) {
	p.add(c)
	p.show(c)
}

func (p *Pages) add(c model.Component) {
	p.AddPage(componentID(c), c, true, true)
}

func (p *Pages) delete(c model.Component) {
	p.RemovePage(componentID(c))
}

func (p *Pages) show(c model.Component) {
	p.SwitchToPage(componentID(c))
}

func componentID(c model.Component) string {
	if c.Name() == "" {
		panic("Component has no name")
	}
	return fmt.Sprintf("%s-%p", c.Name(), c)
}
