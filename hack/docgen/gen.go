package main

import (
	"log"

	"github.com/oam-dev/kubevela/pkg/commands"

	"github.com/spf13/cobra/doc"
)

func main() {
	vela := commands.NewCommand()
	err := doc.GenMarkdownTree(vela, "./docs/en/cli/")
	if err != nil {
		log.Fatal(err)
	}
}
