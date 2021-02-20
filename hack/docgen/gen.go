package main

import (
	"log"

	"github.com/oam-dev/kubevela/references/cli"

	"github.com/spf13/cobra/doc"
)

func main() {
	vela := cli.NewCommand()
	err := doc.GenMarkdownTree(vela, "./docs/en/cli/")
	if err != nil {
		log.Fatal(err)
	}
}
