package view

import (
	"context"
	"strings"

	"github.com/oam-dev/kubevela/references/cli/top/model"
)

// Command is a kind of abstract of user UI operation
type Command struct {
	app *App
}

// NewCommand return app's command instance
func NewCommand(app *App) *Command {
	command := &Command{
		app: app,
	}
	return command
}

// Init command instance
func (c *Command) Init() {}

func (c *Command) exec(_ string, component model.Component) {
	c.app.inject(component)
}

func (c *Command) run(ctx context.Context, cmd string) {
	cmd = strings.ToLower(cmd)
	var component model.Component
	switch {
	case cmd == "?" || cmd == "h" || cmd == "help":
		component = NewHelpView(c.app)
	default:
		if resource, ok := ResourceMap[cmd]; ok {
			component = resource.viewFunc(ctx, c.app)
		} else {
			return
		}
	}
	c.exec(cmd, component)
}
