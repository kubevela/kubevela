/*
Copyright 2023 The KubeVela Authors.

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

package cuegen

import (
	"fmt"
	"strings"

	"golang.org/x/tools/go/packages"
)

// loadPackage loads a package from given path.
func loadPackage(p string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedModule,
	}

	pkgs, err := packages.Load(cfg, []string{p}...)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("expected one package, got %d", len(pkgs))
	}

	// only need to check the first package
	pkg := pkgs[0]
	if pkg.Errors != nil {
		errs := make([]string, 0, len(pkg.Errors))
		for _, e := range pkg.Errors {
			errs = append(errs, fmt.Sprintf("\t%s: %v", pkg.PkgPath, e))
		}
		return nil, fmt.Errorf("could not load Go packages:\n%s", strings.Join(errs, "\n"))
	}

	return pkg, nil
}
