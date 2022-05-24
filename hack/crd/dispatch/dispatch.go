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
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	oldCRD = map[string]bool{"components": true, "applicationconfigurations": true}
	// when controller need to run in runtime cluster, just add them in this map, key=crdName, value=subPath
	runtimeCRD = map[string]string{"rollouts": "rollout"}
	minimalCRD = map[string]bool{"applicationrevisions": true, "applications": true, "definitionrevisions": true, "healthscopes": true,
		"policydefinitions": true, "resourcetrackers": true, "scopedefinitions": true, "traitdefinitions": true, "workflowstepdefinitions": true,
		"workloaddefinitions": true, "rollouts": true}
)

func main() {
	var dir string
	var newDir string
	var minimalDir string
	var runtimeDir string
	if len(os.Args) > 2 {
		dir = os.Args[1]
		newDir = os.Args[2]
		runtimeDir = os.Args[3]
		minimalDir = os.Args[4]
	} else {
		log.Fatal(fmt.Errorf("not enough args"))
	}

	writeNew := func(fileName string, data []byte) {
		pathNew := fmt.Sprintf("%s/%s", newDir, fileName)
		/* #nosec */
		if err := os.WriteFile(pathNew, data, 0644); err != nil {
			log.Fatal(err)
		}
	}

	writeMinimal := func(fileName string, data []byte) {
		pathMinimal := fmt.Sprintf("%s/%s", minimalDir, fileName)
		/* #nosec */
		if err := os.WriteFile(pathMinimal, data, 0644); err != nil {
			log.Fatal(err)
		}
	}

	writeRuntime := func(subPath, fileName string, data []byte) {
		pathRuntime := fmt.Sprintf("%s/%s/charts/crds/%s", runtimeDir, subPath, fileName)
		/* #nosec */
		if err := os.WriteFile(pathRuntime, data, 0644); err != nil {
			log.Fatal(err)
		}
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, _ error) error {
		if info.IsDir() {
			return nil
		}
		resourceName := extractMainInfo(info.Name())
		/* #nosec */
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			return err
		}
		if oldCRD[resourceName] {
			return nil
		}
		if minimalCRD[resourceName] {
			writeMinimal(info.Name(), data)
		}
		if subPath, exist := runtimeCRD[resourceName]; exist {
			writeRuntime(subPath, info.Name(), data)
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
