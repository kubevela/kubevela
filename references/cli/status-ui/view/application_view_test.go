package view

import (
	"testing"

	"github.com/rivo/tview"
)

func TestNewApplicationView(t *testing.T) {
	v := NewApplicationView(nil, ListApplications(make(argMap)))
	v.Init()
	tview.NewApplication().SetRoot(v, true).Run()
}
