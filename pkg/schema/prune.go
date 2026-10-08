/*
Copyright 2026 The KubeVela Authors.

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

package schema

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/oam-dev/kubevela/pkg/cue/process"
)

// ErrUnprunable reports a template whose `parameter` cannot be separated from
// the rest of it by reading the syntax alone.
var ErrUnprunable = errors.New("parameter cannot be pruned from template")

// PruneToParameter reduces a CUE template to its top-level `parameter`
// declarations and the top-level declarations and imports they reference.
//
// Everything else (output, outputs, patch, and the imports only they use) is
// dropped, so it cannot fail the compile that reads the parameter schema.
// Comments stay attached, since the schema's descriptions come from them.
func PruneToParameter(src string) (string, error) {
	return PruneTo(src, process.ParameterFieldName)
}

// PruneTo is PruneToParameter keeping the named top-level fields, such as a
// SourceDefinition's `parameter` and `schema`.
func PruneTo(src string, fields ...string) (string, error) {
	keep := map[string]bool{}
	for _, name := range fields {
		keep[name] = true
	}
	f, err := parser.ParseFile("template", src, parser.ParseComments)
	if err != nil {
		return "", err
	}

	named := map[string][]ast.Decl{}
	imports := map[string]*ast.ImportSpec{}
	var roots []ast.Decl
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.ImportDecl:
			for _, spec := range d.Specs {
				imports[importName(spec)] = spec
			}
		case *ast.Field:
			name, _, err := ast.LabelName(fieldLabel(d.Label))
			if err != nil {
				// A dynamic label could name a kept field.
				return "", fmt.Errorf("%w: dynamic top-level label", ErrUnprunable)
			}
			if keep[name] {
				roots = append(roots, d)
			}
			named[name] = append(named[name], d)
			if a, ok := d.Label.(*ast.Alias); ok {
				named[a.Ident.Name] = append(named[a.Ident.Name], d)
			}
		case *ast.LetClause:
			named[d.Ident.Name] = append(named[d.Ident.Name], d)
		case *ast.Alias:
			named[d.Ident.Name] = append(named[d.Ident.Name], d)
		case *ast.Comprehension:
			if declaresAny(d.Value, keep) {
				return "", fmt.Errorf("%w: kept field declared inside a comprehension", ErrUnprunable)
			}
		case *ast.EmbedDecl:
			return "", fmt.Errorf("%w: top-level embedding", ErrUnprunable)
		}
	}

	kept := map[ast.Decl]bool{}
	usedImports := map[string]bool{}
	queue := append([]ast.Decl(nil), roots...)
	for len(queue) > 0 {
		d := queue[0]
		queue = queue[1:]
		if kept[d] {
			continue
		}
		kept[d] = true
		for name := range references(d) {
			if _, ok := imports[name]; ok {
				usedImports[name] = true
			}
			queue = append(queue, named[name]...)
		}
	}

	out := &ast.File{}
	var specs []*ast.ImportSpec
	for _, d := range f.Decls {
		if id, ok := d.(*ast.ImportDecl); ok {
			for _, spec := range id.Specs {
				if usedImports[importName(spec)] {
					specs = append(specs, spec)
				}
			}
		}
	}
	if len(specs) > 0 {
		out.Decls = append(out.Decls, &ast.ImportDecl{Specs: specs})
	}
	for _, d := range f.Decls {
		if kept[d] {
			out.Decls = append(out.Decls, d)
		}
	}
	b, err := format.Node(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// references collects the top-level identifiers an expression reads. Field
// labels, selector names and the names a clause or alias binds are not reads,
// so they are skipped: `output: image: x` reads `x`, not `output` or `image`.
// A read the parser resolved to a nested scope, such as a comprehension
// variable, is not top-level.
func references(n ast.Node) map[string]bool {
	refs := map[string]bool{}
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		ast.Walk(n, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Field:
				switch l := fieldLabel(n.Label).(type) {
				case *ast.Ident, *ast.BasicLit:
				default:
					walk(l)
				}
				walk(n.Value)
				return false
			case *ast.SelectorExpr:
				walk(n.X)
				return false
			case *ast.ForClause:
				walk(n.Source)
				return false
			case *ast.LetClause:
				walk(n.Expr)
				return false
			case *ast.Alias:
				walk(n.Expr)
				return false
			case *ast.Ident:
				if _, file := n.Scope.(*ast.File); file || n.Scope == nil {
					refs[n.Name] = true
				}
			}
			return true
		}, nil)
	}
	walk(n)
	return refs
}

// declaresAny reports whether a comprehension body declares one of the kept
// fields at the level it is embedded, including through nested comprehensions.
func declaresAny(e ast.Expr, keep map[string]bool) bool {
	s, ok := e.(*ast.StructLit)
	if !ok {
		return true
	}
	for _, el := range s.Elts {
		switch el := el.(type) {
		case *ast.Field:
			name, _, err := ast.LabelName(fieldLabel(el.Label))
			if err != nil || keep[name] {
				return true
			}
		case *ast.Comprehension:
			if declaresAny(el.Value, keep) {
				return true
			}
		case *ast.EmbedDecl:
			return true
		}
	}
	return false
}

func fieldLabel(l ast.Label) ast.Label {
	if a, ok := l.(*ast.Alias); ok {
		if inner, ok := a.Expr.(ast.Label); ok {
			return inner
		}
	}
	return l
}

// importName is the identifier an import is referenced by: its alias, or the
// last path element with any `:pkg` qualifier or version removed.
func importName(spec *ast.ImportSpec) string {
	if spec.Name != nil {
		return spec.Name.Name
	}
	p, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		p = strings.Trim(spec.Path.Value, `"`)
	}
	if i := strings.LastIndex(p, ":"); i >= 0 {
		return p[i+1:]
	}
	p, _, _ = strings.Cut(p, "@")
	return path.Base(p)
}

// ParameterSchemaPath reports how ParseParameterSchema compiled a template.
type ParameterSchemaPath string

const (
	// PathPlain compiled the pruned template with plain CUE.
	PathPlain ParameterSchemaPath = "plain"
	// PathCuex compiled the pruned template with the cuex compiler, because
	// `parameter` itself needs a package plain CUE does not have.
	PathCuex ParameterSchemaPath = "cuex"
	// PathFull compiled the whole template, as ParsePropertiesToSchema does.
	PathFull ParameterSchemaPath = "full"
)

// ParseParameterSchema is ParsePropertiesToSchema reading only `parameter` and
// what it references. A template it cannot prune is compiled whole.
func ParseParameterSchema(ctx context.Context, s string) (*openapi3.Schema, ParameterSchemaPath, error) {
	pruned, err := PruneToParameter(s)
	if err != nil {
		sch, err := ParsePropertiesToSchema(ctx, s)
		return sch, PathFull, err
	}
	src := pruned + "\n" + BaseTemplate
	if val := cuecontext.New().CompileString(src); val.Err() == nil {
		sch, err := ParseValueToSchema(val)
		return sch, PathPlain, err
	}
	sch, err := ParsePropertiesToSchema(ctx, pruned)
	return sch, PathCuex, err
}
