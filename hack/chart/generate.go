package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openservicemesh/osm/pkg/cli"
)

func main() {
	// Path relative to the Makefile where this is invoked.
	chartPath := filepath.Join("charts", "vela-core")
	source, err := cli.GetChartSource(chartPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting chart source:", err)
		os.Exit(1)
	}
	fmt.Print(source)
}
