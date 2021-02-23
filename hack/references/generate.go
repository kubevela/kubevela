package main

import (
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/references/plugins"
)

func main() {
	ref := &plugins.MarkdownReference{}
	if err := ref.GenerateReferenceDocs(plugins.BaseRefPath); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
