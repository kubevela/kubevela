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
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/oam-dev/kubevela/hack/utils"

	"github.com/mholt/archiver/v3"
)

func main() {
	tgz := archiver.NewTarGz()
	defer tgz.Close()
	var archiveDir, output string
	flag.StringVar(&archiveDir, "path", "references/dashboard/dist", "specify frontend static file")
	flag.StringVar(&output, "output", "", "specify output dir, if not set, output base64 result of the gzip result")
	flag.Parse()
	var stdout bool
	if output == "" {
		stdout = true
		output = fmt.Sprintf("vela-frontend-%d.tgz", time.Now().Nanosecond())
	}
	err := tgz.Archive([]string{archiveDir}, output)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if stdout {
		data, err := ioutil.ReadFile(output)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		PrintToFile(base64.StdEncoding.EncodeToString(data))
		_ = os.Remove(output)
	}
}

func PrintToFile(data string) {
	var buffer bytes.Buffer
	buffer.WriteString(`package fake
var FrontendSource = "`)
	utils.FprintZipData(&buffer, []byte(data))
	buffer.WriteString(`"`)
	_ = ioutil.WriteFile("references/cmd/cli/fake/source.go", buffer.Bytes(), 0644)
}
