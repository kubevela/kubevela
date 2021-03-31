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
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	var crds []string
	args := os.Args
	if len(args) <= 1 {
		fmt.Println("no CRDs is specified")
		os.Exit(1)
	}
	crds = args[1:]
	if err := fixNewSchemaValidationCheck(crds); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func fixNewSchemaValidationCheck(crds []string) error {
	for _, crd := range crds {
		data, err := ioutil.ReadFile(crd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading CRD file %s hit an issue: %s\n", crd, err)
			return err
		}
		var newData []string
		// temporarily corrects spec.validation.openAPIV3Schema issue https://github.com/kubernetes/kubernetes/issues/91395
		if strings.HasSuffix(crd, "charts/vela-core/crds/standard.oam.dev_podspecworkloads.yaml") {
			var previousLine string
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(previousLine, "protocol:") &&
					strings.Contains(line, "description: Protocol for port. Must be UDP, TCP,") {
					tmp := strings.Split(line, "description")

					if len(tmp) > 0 {
						blanks := tmp[0]
						defaultStr := blanks + "default: TCP"
						newData = append(newData, defaultStr)
					}
				}
				newData = append(newData, line)
				previousLine = line
			}
			ioutil.WriteFile(crd, []byte(strings.Join(newData, "\n")), 0644)
		}
	}
	return nil
}
