package main

import (
	"math/rand"
	"os"
	"time"

	"github.com/oam-dev/kubevela/references/cli"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	command := cli.NewCommand()

	if err := command.Execute(); err != nil {
		os.Exit(1)
	}
}
