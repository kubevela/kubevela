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
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	common = map[string]bool{"workloaddefinitions": true, "traitdefinitions": true, "scopedefinitions": true, "healthscopes": true,
		"manualscalertraits": true, "containerizedworkloads": true}
	oldCRD = map[string]bool{"components": true, "applicationconfigurations": true}
)

func main() {
	var dir string
	var oldDir string
	var newDir string
	if len(os.Args) > 2 {
		dir = os.Args[1]
		newDir = os.Args[2]
		oldDir = os.Args[3]
	} else {
		log.Fatal(fmt.Errorf("not enough args"))
	}

	writeOld := func(fileName string, data []byte) {
		pathOld := fmt.Sprintf("%s/%s", oldDir, fileName)
		/* #nosec */
		if err := ioutil.WriteFile(pathOld, data, 0644); err != nil {
			log.Fatal(err)
		}
	}

	writeNew := func(fileName string, data []byte) {
		pathNew := fmt.Sprintf("%s/%s", newDir, fileName)
		/* #nosec */
		if err := ioutil.WriteFile(pathNew, data, 0644); err != nil {
			log.Fatal(err)
		}
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		resourceName := extractMainInfo(info.Name())
		/* #nosec */
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			return err
		}
		if oldCRD[resourceName] {
			writeOld(info.Name(), data)
			return nil
		}
		if common[resourceName] {
			writeOld(info.Name(), data)
		}
		writeNew(info.Name(), data)
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}
	log.Println("complete crd files dispatch")
}

func extractMainInfo(fileName string) string {
	return strings.Split(strings.Split(fileName, "_")[1], ".")[0]
}
