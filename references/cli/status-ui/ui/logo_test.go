package ui

import (
	"testing"

	"github.com/rivo/tview"

	"github.com/oam-dev/kubevela/references/cli/status-ui/config"
)

func TestNewLogo(t *testing.T) {
	app := tview.NewApplication()
	logo := NewLogo(&config.Style{})

	app.SetRoot(logo, true)
	app.Run()
}
