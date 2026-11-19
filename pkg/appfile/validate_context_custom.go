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

package appfile

import (
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"github.com/pkg/errors"
)

// contextCustomPaths returns every static path a template reads under
// context.custom, the data Application-scoped policies supply.
//
// A dynamic index ends the path at the last key known without evaluating, and
// a template that does not parse contributes nothing: compiling it reports
// that error.
func contextCustomPaths(templates ...string) [][]string {
	seen := map[string][]string{}
	for _, tmpl := range templates {
		f, err := parser.ParseFile("", tmpl)
		if err != nil {
			continue
		}
		var visit func(ast.Node) bool
		visit = func(n ast.Node) bool {
			switch n.(type) {
			case *ast.SelectorExpr, *ast.IndexExpr:
			default:
				return true
			}
			chain, ok := referenceChain(n)
			if !ok || len(chain) < 3 || chain[0] != "context" || chain[1] != "custom" {
				// Not a whole chain from context.custom: an index inside it
				// may still read one.
				return true
			}
			seen[strings.Join(chain[2:], "\x00")] = chain[2:]
			walkDynamicIndexes(n, visit)
			return false
		}
		ast.Walk(f, visit, nil)
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	paths := make([][]string, 0, len(keys))
	for _, k := range keys {
		paths = append(paths, seen[k])
	}
	return paths
}

// referenceChain flattens a selector or index expression to its labels, root
// first. The chain ends at the first index that is not a string literal, and
// nothing selected after it is added; ok is false when the root is not an
// identifier.
func referenceChain(n ast.Node) (chain []string, ok bool) {
	chain, _, ok = referenceLabels(n)
	return chain, ok
}

// referenceLabels is referenceChain with the source position of each label.
func referenceLabels(n ast.Node) (chain []string, at []token.Pos, ok bool) {
	chain, at, _, ok = labelsUpToIndex(n)
	return chain, at, ok
}

// labelsUpToIndex is referenceLabels, also reporting whether the chain ended at
// an index that is not a string literal.
func labelsUpToIndex(n ast.Node) (chain []string, at []token.Pos, ended, ok bool) {
	switch x := n.(type) {
	case *ast.Ident:
		return []string{x.Name}, []token.Pos{x.Pos()}, false, true
	case *ast.SelectorExpr:
		chain, at, ended, ok = labelsUpToIndex(x.X)
		if !ok || ended {
			return chain, at, ended, ok
		}
		name, _, err := ast.LabelName(x.Sel)
		if err != nil {
			return chain, at, true, true
		}
		return append(chain, name), append(at, x.Sel.Pos()), false, true
	case *ast.IndexExpr:
		chain, at, ended, ok = labelsUpToIndex(x.X)
		if !ok || ended {
			return chain, at, ended, ok
		}
		lit, isLit := x.Index.(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return chain, at, true, true
		}
		name, err := strconv.Unquote(lit.Value)
		if err != nil {
			return chain, at, true, true
		}
		return append(chain, name), append(at, lit.Pos()), false, true
	}
	return nil, nil, false, false
}

// walkDynamicIndexes walks, with visit, each index of a selector or index
// chain that is not a string literal: a read there is a read of its own.
func walkDynamicIndexes(n ast.Node, visit func(ast.Node) bool) {
	for {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			n = x.X
		case *ast.IndexExpr:
			if lit, isLit := x.Index.(*ast.BasicLit); !isLit || lit.Kind != token.STRING {
				ast.Walk(x.Index, visit, nil)
			}
			n = x.X
		default:
			return
		}
	}
}

// srcPos is a line and column in one source file.
type srcPos struct{ line, column int }

// contextCustomReadsAt maps the position of each key in a context.custom read
// in src to the path read up to and including that key, so an error CUE
// reports at a key names exactly the read it failed on.
func contextCustomReadsAt(src string) map[srcPos][]string {
	f, err := parser.ParseFile("", src)
	if err != nil {
		return nil
	}
	reads := map[srcPos][]string{}
	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch n.(type) {
		case *ast.SelectorExpr, *ast.IndexExpr:
		default:
			return true
		}
		chain, at, ok := referenceLabels(n)
		if !ok || len(chain) < 3 || chain[0] != "context" || chain[1] != "custom" {
			return true
		}
		for i := 2; i < len(chain); i++ {
			reads[srcPos{at[i].Line(), at[i].Column()}] = chain[2 : i+1]
		}
		walkDynamicIndexes(n, visit)
		return false
	}
	ast.Walk(f, visit, nil)
	return reads
}

// contextCustomPlaceholders declares each path a template reads under
// context.custom as top, for admission only.
//
// Admission validates before any policy runs, so the keys do not exist and a
// parameter defaulted from one, `*context.custom.team | string`, loses its
// default and is reported missing. Top keeps the default, and unifies away
// against anything the template declares for the key itself.
func contextCustomPlaceholders(templates ...string) string {
	var b strings.Builder
	for _, path := range contextCustomPaths(templates...) {
		b.WriteString("\ncontext: custom: ")
		for _, label := range path {
			b.WriteString(strconv.Quote(label))
			b.WriteString(": ")
		}
		b.WriteString("_")
	}
	return b.String()
}

// contextDefaultedParams returns the dotted paths of the parameters a template
// defaults from policy context, `team: *context.custom.team | string`.
//
// Read from the declaration rather than the evaluated schema: admission has no
// policy context, and a type the template gives the key collapses the
// disjunction to one without a default.
func contextDefaultedParams(template string) map[string]bool {
	f, err := parser.ParseFile("", template)
	if err != nil {
		return nil
	}
	c := defaultsCollector{defs: map[string]ast.Expr{}, out: map[string]bool{}, visiting: map[string]bool{}}
	var params []ast.Expr
	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil {
			continue
		}
		if name == "parameter" {
			params = append(params, field.Value)
		} else if strings.HasPrefix(name, "#") {
			c.defs[name] = field.Value
		}
	}
	for _, v := range params {
		c.collect("", v)
	}
	return c.out
}

// defaultsCollector walks a parameter declaration, following the file's
// top-level definitions it refers to, unifies with or embeds.
type defaultsCollector struct {
	defs     map[string]ast.Expr
	out      map[string]bool
	visiting map[string]bool
}

// collect records prefix when v, or a definition it names, is a disjunction defaulted from
// context.custom, and walks into the fields of the structs it reaches.
func (c defaultsCollector) collect(prefix string, v ast.Expr) {
	if prefix != "" && defaultReadsContextCustom(v) {
		c.out[prefix] = true
		return
	}
	switch x := v.(type) {
	case *ast.Ident:
		def, ok := c.defs[x.Name]
		if !ok || c.visiting[x.Name] {
			return
		}
		c.visiting[x.Name] = true
		c.collect(prefix, def)
		delete(c.visiting, x.Name)
	case *ast.BinaryExpr:
		if x.Op == token.AND {
			c.collect(prefix, x.X)
			c.collect(prefix, x.Y)
		}
	case *ast.ParenExpr:
		c.collect(prefix, x.X)
	case *ast.StructLit:
		for _, elt := range x.Elts {
			switch e := elt.(type) {
			case *ast.EmbedDecl:
				c.collect(prefix, e.Expr)
			case *ast.Field:
				name, _, err := ast.LabelName(e.Label)
				if err != nil {
					continue
				}
				path := name
				if prefix != "" {
					path = prefix + "." + name
				}
				c.collect(path, e.Value)
			}
		}
	}
}

// defaultReadsContextCustom reports whether a disjunction's marked default
// reads context.custom.
func defaultReadsContextCustom(v ast.Expr) bool {
	switch x := v.(type) {
	case *ast.BinaryExpr:
		if x.Op != token.OR {
			return false
		}
		return defaultReadsContextCustom(x.X) || defaultReadsContextCustom(x.Y)
	case *ast.UnaryExpr:
		return x.Op == token.MUL && readsContextCustom(x.X)
	case *ast.ParenExpr:
		return defaultReadsContextCustom(x.X)
	}
	return false
}

func readsContextCustom(n ast.Node) bool {
	found := false
	ast.Walk(n, func(n ast.Node) bool {
		if found {
			return false
		}
		chain, ok := referenceChain(n)
		if ok && len(chain) >= 2 && chain[0] == "context" && chain[1] == "custom" {
			found = true
			return false
		}
		return true
	}, nil)
	return found
}

// withoutContextDefaulted drops, from a missing-parameter error, the
// parameters the template defaults from policy context. It returns nil when
// none are left.
func withoutContextDefaulted(err error, defaulted map[string]bool) error {
	var missing *missingParamsError
	if len(defaulted) == 0 || !errors.As(err, &missing) {
		return err
	}
	var left []string
	for _, name := range missing.names {
		if !coveredByPath(name, defaulted) {
			left = append(left, name)
		}
	}
	if len(left) == 0 {
		return nil
	}
	return &missingParamsError{names: left}
}

// coveredByPath reports whether name is one of paths or lies beneath one.
func coveredByPath(name string, paths map[string]bool) bool {
	for p := range paths {
		if name == p || strings.HasPrefix(name, p+".") {
			return true
		}
	}
	return false
}
