package main

import (
	"log"

	"github.com/oam-dev/kubevela/pkg/apiserver/commands"
	"github.com/oam-dev/kubevela/pkg/apiserver/commands/server"
)

func main() {
	app := commands.NewCLI(
		"velacp",
		"KubeVela control plane",
	)
	app.AddCommands(
		server.NewServerCommand(),
	)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
