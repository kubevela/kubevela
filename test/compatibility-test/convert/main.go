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

package main // #nosec

// generate compatibility testdata
import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var srcdir, dstdir string
	if len(os.Args) > 1 {
		srcdir = os.Args[1]
		dstdir = os.Args[2]
	}
	err := filepath.Walk(srcdir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
		}
		if info.IsDir() {
			return nil
		}
		/* #nosec */
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to read file", err)
			return err
		}
		fileName := info.Name()
		var newdata string
		if fileName == "core.oam.dev_workloaddefinitions.yaml" || fileName == "core.oam.dev_traitdefinitions.yaml" || fileName == "core.oam.dev_scopedefinitions.yaml" {
			newdata = strings.ReplaceAll(string(data), "scope: Namespaced", "scope: Cluster")
		} else {
			newdata = string(data)
		}
		dstpath := dstdir + "/" + fileName
		/* #nosec */
		if err = ioutil.WriteFile(dstpath, []byte(newdata), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "failed to write file:", err)
			return err
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
