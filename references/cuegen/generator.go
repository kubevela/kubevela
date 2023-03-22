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
	goast "go/ast"
	gotypes "go/types"
	"io"
	"strings"

	cueast "cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	cueformat "cuelang.org/go/cue/format"
	"golang.org/x/tools/go/packages"
)

// Generator generates CUE schema from Go struct.
type Generator struct {
	// immutable
	pkg   *packages.Package
	types typeInfo

	opts *options
}

// NewGenerator creates a new generator with given file or package path.
func NewGenerator(f string) (*Generator, error) {
	pkg, err := loadPackage(f)
	if err != nil {
		return nil, err
	}

	types := getTypeInfo(pkg)

	g := &Generator{
		pkg:   pkg,
		types: types,
		opts:  defaultOptions,
	}

	return g, nil
}

// Generate generates CUE schema from Go struct and writes to w.
// And it can be called multiple times with different options.
//
// NB: it's not thread-safe.
func (g *Generator) Generate(w io.Writer, opts ...Option) error {
	g.opts = defaultOptions // reset options for each call
	for _, opt := range opts {
		if opt != nil {
			opt(g.opts)
		}
	}

	var decls []cueast.Decl
	for _, syntax := range g.pkg.Syntax {
		for _, decl := range syntax.Decls {
			if d, ok := decl.(*goast.GenDecl); ok {
				t, err := g.convertDecls(d)
				if err != nil {
					return err
				}
				decls = append(decls, t...)
			}
		}
	}

	pkg := &cueast.Package{Name: ident(g.pkg.Name, false)}

	f := &cueast.File{Decls: []cueast.Decl{pkg}}
	f.Decls = append(f.Decls, decls...)

	return g.write(w, f)
}

func (g *Generator) write(w io.Writer, f *cueast.File) error {
	if w == nil {
		return fmt.Errorf("nil writer")
	}

	if err := astutil.Sanitize(f); err != nil {
		return err
	}

	b, err := cueformat.Node(f, cueformat.Simplify())
	if err != nil {
		return err
	}

	_, err = w.Write(b)
	return err
}

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

type typeInfo map[gotypes.Type]*goast.StructType

func getTypeInfo(p *packages.Package) typeInfo {
	m := make(typeInfo)

	for _, f := range p.Syntax {
		goast.Inspect(f, func(n goast.Node) bool {
			// record all struct types
			if t, ok := n.(*goast.StructType); ok {
				m[p.TypesInfo.TypeOf(t)] = t
			}
			return true
		})
	}

	return m
}
