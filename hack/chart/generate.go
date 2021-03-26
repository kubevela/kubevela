/*
Copyright 2021 The KubeVela Authors.

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
