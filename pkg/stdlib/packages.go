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
	"os"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/parser"
	"k8s.io/klog/v2"
)

func init() {
	var err error
	BuiltinImports, err = initBuiltinImports()
	if err != nil {
		klog.ErrorS(err, "Unable to init builtin imports")
		os.Exit(1)
	}
}

var (
	//go:embed pkgs op.cue ql.cue
	fs embed.FS
	// BuiltinImports is the builtin imports for cue
	BuiltinImports []*build.Instance
)

// GetPackages Get Stdlib packages
func GetPackages() (map[string]string, error) {
	files, err := fs.ReadDir("pkgs")
	if err != nil {
		return nil, err
	}

	opBytes, err := fs.ReadFile("op.cue")
	if err != nil {
		return nil, err
	}

	qlBytes, err := fs.ReadFile("ql.cue")
	if err != nil {
		return nil, err
	}

	opContent := string(opBytes) + "\n"
	qlContent := string(qlBytes) + "\n"
	for _, file := range files {
		body, err := fs.ReadFile("pkgs/" + file.Name())
		if err != nil {
			return nil, err
		}
		pkgContent := fmt.Sprintf("%s: {\n%s\n}\n", strings.TrimSuffix(file.Name(), ".cue"), string(body))
		opContent += pkgContent
		if file.Name() == "kube.cue" || file.Name() == "query.cue" {
			qlContent += pkgContent
		}
	}

	return map[string]string{
		"vela/op": opContent,
		"vela/ql": qlContent,
	}, nil
}

// AddImportsFor install imports for build.Instance.
func AddImportsFor(inst *build.Instance, tagTempl string) error {
	inst.Imports = append(inst.Imports, BuiltinImports...)
	if tagTempl != "" {
		p := &build.Instance{
			PkgName:    filepath.Base("vela/custom"),
			ImportPath: "vela/custom",
		}
		file, err := parser.ParseFile("-", tagTempl, parser.ParseComments)
		if err != nil {
			return err
		}
		if err := p.AddSyntax(file); err != nil {
			return err
		}
		inst.Imports = append(inst.Imports, p)
	}
	return nil
}

func initBuiltinImports() ([]*build.Instance, error) {
	imports := make([]*build.Instance, 0)
	pkgs, err := GetPackages()
	if err != nil {
		return nil, err
	}
	for path, content := range pkgs {
		p := &build.Instance{
			PkgName:    filepath.Base(path),
			ImportPath: path,
		}
		file, err := parser.ParseFile("-", content, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		if err := p.AddSyntax(file); err != nil {
			return nil, err
		}
		imports = append(imports, p)
	}
	return imports, nil
}
