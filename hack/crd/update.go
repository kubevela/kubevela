package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	err = FixNewSchemaValidationCheck(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error getting chart source:", err)
		os.Exit(1)
	}
}

// FixNewSchemaValidationCheck temporarily corrects spec.validation.openAPIV3Schema issue, and it would be removed
// after this issue was fixed https://github.com/oam-dev/kubevela/issues/284.
func FixNewSchemaValidationCheck(chartPath string) error {
	err := filepath.Walk(chartPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to list the content of", path)
			return err
		}
		if info.IsDir() {
			return nil
		}

		if info.Name() != "standard.oam.dev_podspecworkloads.yaml" {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "open path err", path, err)
			return err
		}
		var newdata []string
		var previousLine string
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(previousLine, "protocol:") &&
				strings.Contains(line, "description: Protocol for port. Must be UDP, TCP,") {
				tmp := strings.Split(line, "description")

				if len(tmp) > 0 {
					blanks := tmp[0]
					defaultStr := blanks + "default: TCP"
					newdata = append(newdata, defaultStr)
				}
			}
			newdata = append(newdata, line)
			previousLine = line
		}

		return ioutil.WriteFile(path, []byte(strings.Join(newdata, "\n")), info.Mode())
	})
	return err
}
