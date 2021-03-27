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
	"path/filepath"
	"strings"
)

func main() {
	filepath.Walk("docs/en/", func(path string, info os.FileInfo, err error) error {
		fmt.Println(path, info.IsDir())
		if info.IsDir() {
			return nil
		}
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Println("read file err", path, err)
			return nil
		}
		lines := strings.Split(string(data), "\n")
		if !strings.HasPrefix(lines[0], "#") || !strings.HasSuffix(path, ".md") {
			return nil
		}
		fmt.Println("XXXX", lines[0])

		lines[0] = strings.Replace(lines[0], "#", "title: ", 1)
		lines[0] = "---\n" + lines[0] + "\n---"
		if err = ioutil.WriteFile(path, []byte(strings.Join(lines, "\n")), info.Mode()); err != nil {
			fmt.Println("write file", path, err)
		}
		return nil
	})
}
