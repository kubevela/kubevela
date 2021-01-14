package main

import (
	"fmt"
	"os"

	"github.com/oam-dev/kubevela/pkg/plugins"
)

func main() {
	if err := plugins.GenerateReferenceDocs(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
