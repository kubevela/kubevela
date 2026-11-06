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

package ctxschema

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueformat "cuelang.org/go/cue/format"
)

// Policy is an Application-scoped policy that applies to an Application, in
// the order the controller applies them.
type Policy struct {
	Name     string
	Template string
	// Unknown marks a policy whose template admission cannot have, so it may
	// supply anything.
	Unknown bool
}

// View is context.custom as an Application's components will see it, built
// from the schemas of the policies that apply, without evaluating them.
type View struct {
	// Complete is whether the view names every key context.custom can hold:
	// every policy that may set ctx declares a closed schema, and none can
	// add policies admission does not see.
	Complete bool
	// Warnings name the policies whose template or schema could not be read;
	// they count as supplying anything.
	Warnings []string

	keys    map[string][]declaration
	imports map[string]string // import path -> alias
}

// declaration is one policy's type for a top-level key. The controller sets a
// key from the last policy that sets it, so a later declaration overrides an
// earlier one, unless it may not set the key at all.
type declaration struct {
	policy   string
	expr     string
	optional bool
	// imports are the paths the expression uses, under their aliases.
	imports []string
}

// Build builds the view from the policies that apply, in order.
func Build(policies []Policy) *View {
	v := &View{Complete: true, keys: map[string][]declaration{}, imports: map[string]string{}}
	cctx := cuecontext.New()
	for _, p := range policies {
		v.add(cctx, p)
	}
	return v
}

func (v *View) add(cctx *cue.Context, p Policy) {
	if p.Unknown {
		v.Complete = false
		return
	}
	info, err := Read(p.Template)
	if err != nil {
		v.unreadable(p.Name, "cannot read its template", err)
		return
	}
	if info.SchemaUnread {
		v.unreadable(p.Name, "declares a schema admission cannot read from its syntax", errors.New("schema.ctx must be a struct literal field of a top-level schema struct"))
		return
	}
	if info.SetsPolicies {
		v.Complete = false
	}
	if info.Schema == "" {
		if info.SetsCtx {
			v.Complete = false
		}
		return
	}
	schema, err := Compile(cctx, info.Schema)
	if err != nil {
		v.unreadable(p.Name, "cannot read schema.ctx", err)
		return
	}
	if !isClosedWithoutPatterns(schema) {
		v.Complete = false
	}
	it, err := schema.Fields(cue.Optional(true))
	if err != nil {
		v.unreadable(p.Name, "cannot read schema.ctx", err)
		return
	}
	for it.Next() {
		expr, imports, err := v.render(it.Value())
		if err != nil {
			v.unreadable(p.Name, "cannot read schema.ctx", err)
			return
		}
		name := it.Selector().Unquoted()
		v.keys[name] = append(v.keys[name], declaration{
			policy:   p.Name,
			expr:     expr,
			optional: info.Conditional || it.Selector().ConstraintType() == cue.OptionalConstraint,
			imports:  imports,
		})
	}
}

func (v *View) unreadable(policy, what string, err error) {
	v.Complete = false
	v.Warnings = append(v.Warnings, fmt.Sprintf("policy %q: %s, so Applications are not checked against it: %v", policy, what, err))
}

// probeKey is a label no schema declares, to ask whether a schema admits
// undeclared keys.
const probeKey = "__ctxschema_undeclared__"

// isClosedWithoutPatterns reports whether a schema admits only the keys it
// names. A pattern constraint admits keys the view cannot list.
func isClosedWithoutPatterns(schema cue.Value) bool {
	if schema.Allows(cue.Str(probeKey)) {
		return false
	}
	st := structOf(schema.Syntax(cue.Raw()))
	if st == nil {
		return false
	}
	for _, elt := range st.Elts {
		f, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		if _, isPattern := f.Label.(*ast.ListLit); isPattern {
			return false
		}
	}
	return true
}

// structOf unwraps the struct a schema's syntax declares: `close({...})`, a
// plain struct, or either embedded in the file Syntax returns with imports.
func structOf(n ast.Node) *ast.StructLit {
	switch x := n.(type) {
	case *ast.File:
		for _, d := range x.Decls {
			if e, ok := d.(*ast.EmbedDecl); ok {
				return structOf(e.Expr)
			}
		}
	case *ast.CallExpr:
		if len(x.Args) == 1 {
			return structOf(x.Args[0])
		}
	case *ast.StructLit:
		return x
	}
	return nil
}

// render writes a key's type as CUE that stands in another file: each import
// it uses is renamed to an alias no template uses.
func (v *View) render(val cue.Value) (string, []string, error) {
	n := val.Syntax(cue.Raw())
	expr, ok := n.(ast.Expr)
	var imports []string
	if f, isFile := n.(*ast.File); isFile {
		renames := map[string]string{}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return "", nil, err
			}
			imports = append(imports, p)
			name := path.Base(p)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			renames[name] = v.alias(p)
		}
		for _, d := range f.Decls {
			if e, isEmbed := d.(*ast.EmbedDecl); isEmbed {
				expr, ok = e.Expr, true
			}
		}
		if ok {
			ast.Walk(expr, func(n ast.Node) bool {
				if sel, isSel := n.(*ast.SelectorExpr); isSel {
					if id, isIdent := sel.X.(*ast.Ident); isIdent && renames[id.Name] != "" {
						id.Name = renames[id.Name]
						id.Node = nil
					}
				}
				return true
			}, nil)
		}
	}
	if !ok {
		return "", nil, fmt.Errorf("unexpected syntax %T", n)
	}
	b, err := cueformat.Node(expr)
	if err != nil {
		return "", nil, err
	}
	return string(b), imports, nil
}

func (v *View) alias(importPath string) string {
	if a, ok := v.imports[importPath]; ok {
		return a
	}
	a := "__ctxschema_" + strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(importPath)
	v.imports[importPath] = a
	return a
}

// candidates returns the declarations a key may take its value from, and
// whether some policy is certain to set it: the last declaration certain to,
// and every later one that may.
func candidates(decls []declaration) ([]declaration, bool) {
	for i := len(decls) - 1; i >= 0; i-- {
		if !decls[i].optional {
			return decls[i:], true
		}
	}
	return decls, false
}

// Declares reports whether some policy declares the key.
func (v *View) Declares(key string) bool { return len(v.keys[key]) > 0 }

// Optional reports whether a declared key may be absent, and the last policy
// declaring it.
func (v *View) Optional(key string) (policy string, optional bool) {
	decls := v.keys[key]
	if len(decls) == 0 {
		return "", false
	}
	_, required := candidates(decls)
	return decls[len(decls)-1].policy, !required
}

// Source returns the view as CUE to place in a template's file: the imports to
// put before the template's own, and the context.custom declaration to put
// after it. A complete view is closed, so reading a key no policy declares is
// an error.
func (v *View) Source() (imports string, context string) {
	// Only the imports a key the view writes uses: a loosened key uses none, and
	// CUE refuses an import nothing uses.
	used := map[string]bool{}
	for _, decls := range v.keys {
		cands, _ := candidates(decls)
		for _, d := range cands {
			for _, p := range d.imports {
				used[p] = true
			}
		}
	}
	paths := make([]string, 0, len(used))
	for p := range used {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var ib strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&ib, "import %s %q\n", v.imports[p], p)
	}
	return ib.String(), v.context(false)
}

// SourceWithoutImports returns the context.custom declaration for a file that
// cannot take imports, such as a definition another extends: a key typed with
// one is left unchecked.
func (v *View) SourceWithoutImports() string { return v.context(true) }

func (v *View) context(withoutImports bool) string {
	names := make([]string, 0, len(v.keys))
	for k := range v.keys {
		names = append(names, k)
	}
	sort.Strings(names)
	var cb strings.Builder
	for _, k := range names {
		decls, required := candidates(v.keys[k])
		exprs := make([]string, 0, len(decls))
		for _, d := range decls {
			if withoutImports && len(d.imports) > 0 {
				exprs = []string{"_"}
				break
			}
			exprs = append(exprs, "("+d.expr+")")
		}
		marker := ""
		if !required {
			marker = "?"
		}
		fmt.Fprintf(&cb, "\t%s%s: %s\n", strconv.Quote(k), marker, strings.Join(exprs, " | "))
	}
	body := "{\n" + cb.String() + "}"
	if v.Complete {
		body = "close(" + body + ")"
	}
	return "context: custom: " + body + "\n"
}

// Loosen returns a copy of the view leaving one key unchecked, its presence as in
// v, to tell whether that key's type is what a template fails.
func (v *View) Loosen(key string) *View {
	c := *v
	c.keys = make(map[string][]declaration, len(v.keys))
	for k, d := range v.keys {
		c.keys[k] = d
	}
	if decls, ok := v.keys[key]; ok {
		_, required := candidates(decls)
		c.keys[key] = []declaration{{policy: decls[len(decls)-1].policy, expr: "_", optional: !required}}
	}
	return &c
}

// PathOptional reports whether a path under context.custom is declared and
// may be absent, and the last policy declaring its top-level key. An
// undeclared path is neither.
func (v *View) PathOptional(path []string) (policy string, optional bool) {
	if len(path) == 0 {
		return "", false
	}
	policy, optional = v.Optional(path[0])
	if policy == "" || optional {
		return policy, optional
	}
	imports, ctx := v.Source()
	cur := cuecontext.New().CompileString(imports + ctx).LookupPath(cue.ParsePath("context.custom"))
	for _, seg := range path {
		next, segOptional, found := field(cur, seg)
		if !found {
			return "", false
		}
		if segOptional {
			return policy, true
		}
		cur = next
	}
	return policy, false
}

// field finds a field of a struct, required or optional.
func field(v cue.Value, name string) (cue.Value, bool, bool) {
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		return cue.Value{}, false, false
	}
	for it.Next() {
		if it.Selector().Unquoted() == name {
			return it.Value(), it.Selector().ConstraintType() == cue.OptionalConstraint, true
		}
	}
	return cue.Value{}, false, false
}
