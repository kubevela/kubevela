package main

import (
	"math/rand"
	"os"
	"time"

	_ "github.com/oam-dev/kubevela/pkg/builtin/build"
	"github.com/oam-dev/kubevela/pkg/commands"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	command := commands.NewCommand()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
