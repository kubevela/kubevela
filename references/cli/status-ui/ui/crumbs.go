package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Crumbs struct {
	*tview.Flex
}

func NewCrumbs() *Crumbs {
	c := &Crumbs{
		Flex: tview.NewFlex(),
	}
	c.init()
	return c
}

func (c *Crumbs) init() {
}

func (c *Crumbs) StackPop(old, new model.Component) {
	num := c.GetItemCount()
	c.RemoveItem(c.GetItem(num - 1))
	c.RemoveItem(c.GetItem(num - 2))

}

func (c *Crumbs) StackPush(component model.Component) {
	name := component.Name()
	t := tview.NewTextView()
	t.SetBackgroundColor(tcell.ColorOrange)
	t.SetTextAlign(tview.AlignCenter)

	t.SetText(name)
	c.AddItem(t, len(name)+2, 0, false)
	c.AddItem(tview.NewTextView(), 1, 0, false)
}
