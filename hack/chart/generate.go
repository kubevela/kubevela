package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/oam-dev/kubevela/hack/utils"
)

func main() {
	// Path relative to the Makefile where this is invoked.
	chartPath := filepath.Join("charts", "vela-core")
	source, err := utils.GetChartSource(chartPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting chart source:", err)
		os.Exit(1)
	}
	printToFile(source)
}

func printToFile(data string) {
	var buffer bytes.Buffer
	buffer.WriteString(`package fake
var ChartSource = "`)
	utils.FprintZipData(&buffer, []byte(data))
	buffer.WriteString(`"`)
	_ = ioutil.WriteFile("references/cmd/cli/fake/chart_source.go", buffer.Bytes(), 0644)
}
