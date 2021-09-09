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

package stdlib

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/build"
)

var (
	//go:embed pkgs op.cue
	fs         embed.FS
	pkgContent string
)

// GetPackages Get Stdlib packages
func GetPackages(tagTempl string) (map[string]string, error) {

	files, err := fs.ReadDir("pkgs")
	if err != nil {
		return nil, err
	}

	opBytes, err := fs.ReadFile("op.cue")
	if err != nil {
		return nil, err
	}

	pkgContent = string(opBytes) + "\n"
	for _, file := range files {
		body, err := fs.ReadFile("pkgs/" + file.Name())
		if err != nil {
			return nil, err
		}
		pkgContent += fmt.Sprintf("%s: {\n%s\n}\n", strings.TrimSuffix(file.Name(), ".cue"), string(body))
	}

	return map[string]string{
		"vela/op": pkgContent + "\n" + tagTempl,
	}, nil
}

// AddImportsFor install imports for build.Instance.
func AddImportsFor(inst *build.Instance, tagTempl string) error {
	pkgs, err := GetPackages(tagTempl)
	if err != nil {
		return err
	}
	for path, content := range pkgs {
		p := &build.Instance{
			PkgName:    filepath.Base(path),
			ImportPath: path,
		}
		if err := p.AddFile("-", content); err != nil {
			return err
		}
		inst.Imports = append(inst.Imports, p)
	}
	return nil
}
