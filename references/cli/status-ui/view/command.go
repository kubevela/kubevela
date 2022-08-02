package view

import (
	"context"
	"strings"

	"github.com/oam-dev/kubevela/references/cli/status-ui/model"
)

type Command struct {
	app *App
}

func NewCommand(app *App) *Command {
	command := &Command{
		app: app,
	}
	return command
}

func (c *Command) Init() {
}

func (c *Command) exec(cmd string, component model.Component) {
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
