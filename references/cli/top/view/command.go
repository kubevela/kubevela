/*
Copyright 2022 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

func (c *Command) exec(_ string, component model.View) {
	c.app.inject(component)
}

func (c *Command) run(ctx context.Context, cmd string) {
	cmd = strings.ToLower(cmd)
	var component model.View
	switch {
	case cmd == "?" || cmd == "h" || cmd == "help":
		component = NewHelpView(c.app)
	case cmd == "yaml":
		component = NewYamlView(ctx, c.app)
	case cmd == "topology":
		component = NewTopologyView(ctx, c.app)
	case cmd == "log":
		component = NewLogView(ctx, c.app)
	default:
		if resourceView, ok := ResourceViewMap[cmd]; ok {
			resourceView.InitView(ctx, c.app)
			component = resourceView
		} else {
			return
		}
	}
	c.exec(cmd, component)
}
