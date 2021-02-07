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
		// fix issue https://github.com/oam-dev/kubevela/issues/993
		if strings.HasSuffix(crd, "legacy/charts/vela-core-legacy/crds/standard.oam.dev_routes.yaml") {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "default: Issuer") {
					continue
				}
				newData = append(newData, line)
			}
			ioutil.WriteFile(crd, []byte(strings.Join(newData, "\n")), 0644)
		}
	}
	return nil
}
