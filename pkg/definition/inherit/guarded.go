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

package inherit

import (
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// A field an ancestor writes inside an `if` or `for` exists only for some
// Applications. Compiled with nothing bound, as at admission, the field is not
// there, so a child writing it looks like an addition, and then every
// Application that meets the condition is refused at render. Such a child is
// refused when it is written instead.

// guardedField is a leaf an ancestor writes only under a comprehension: the
// path elements leading to it from the surface, a list item as `[0]` and a
// label known only at render as a pattern, and the clauses guarding it as
// written. value is the expression the leaf is set to.
type guardedField struct {
	path  []string
	cond  string
	value ast.Expr
}

// covers reports whether a child leaf at path is the guarded field or lies
// below it.
func (g guardedField) covers(path []string) bool {
	if len(path) < len(g.path) {
		return false
	}
	for i, p := range g.path {
		if !globMatch(p, path[i]) {
			return false
		}
	}
	return true
}

// guardedFields reads, from a template's source, the leaves of its surface
// written inside a comprehension. A template with no `if` or `for` at all is
// not parsed.
func guardedFields(level Level, s Surface) []guardedField {
	if !strings.Contains(level.Template, "if ") && !strings.Contains(level.Template, "for ") {
		return nil
	}
	// A template that will not parse is reported by the checks that compile it;
	// here it only means there is nothing guarded to read.
	file, parseErr := parser.ParseFile(level.Name+".cue", level.Template, parser.ParseComments)
	if parseErr != nil || file == nil {
		file = &ast.File{}
	}
	w := guardWalker{surface: map[string]bool{}}
	for _, f := range s.Fields {
		w.surface[f] = true
	}
	w.decls(file.Decls, nil, "")
	return w.out
}

type guardWalker struct {
	surface map[string]bool
	out     []guardedField
}

// decls walks the declarations of a struct at path at. cond is the clauses
// guarding it, empty when nothing does.
func (w *guardWalker) decls(decls []ast.Decl, at []string, cond string) {
	for _, decl := range decls {
		switch d := decl.(type) {
		case *ast.Field:
			label := finalLabel(d.Label)
			if len(at) == 0 && !w.surface[label] {
				continue
			}
			w.value(d.Value, appendPath(at, label), cond, true)
		case *ast.Comprehension:
			// A guarded or generated field sits at the same depth as the
			// comprehension that produces it.
			w.value(d.Value, at, joinCond(cond, d.Clauses), false)
		case *ast.EmbedDecl:
			// An embedded reference, such as webservice's
			// `labels: {if parameter.labels != _|_ {parameter.labels}}`, declares
			// no field of its own, so only an embedded struct is read.
			w.value(d.Expr, at, cond, false)
		}
	}
}

// value walks a value. Under a guard, the value of a field or list item that is
// not a struct or list literal is a leaf, and so is an empty list, which no
// child list unifies with item by item. declared is false for an embedding,
// which names no field.
func (w *guardWalker) value(e ast.Expr, at []string, cond string, declared bool) {
	switch v := e.(type) {
	case *ast.StructLit:
		w.decls(v.Elts, at, cond)
		return
	case *ast.ParenExpr:
		w.value(v.X, at, cond, declared)
		return
	case *ast.BinaryExpr:
		if v.Op == token.AND {
			w.value(v.X, at, cond, declared)
			w.value(v.Y, at, cond, declared)
			return
		}
	case *ast.ListLit:
		if len(v.Elts) > 0 {
			w.list(v, at, cond)
			return
		}
	}
	if declared && cond != "" && len(at) > 0 {
		w.out = append(w.out, guardedField{path: at, cond: cond, value: e})
	}
}

// list walks a list literal's items, each at its index. Once a comprehension
// has produced items, the index of every item from there on is only known at
// render, so it is a pattern.
func (w *guardWalker) list(l *ast.ListLit, at []string, cond string) {
	known := true
	for i, el := range l.Elts {
		if _, ok := el.(*ast.Ellipsis); ok {
			continue
		}
		c, isComp := el.(*ast.Comprehension)
		known = known && !isComp
		idx := fmt.Sprintf("[%d]", i)
		if !known {
			idx = "[*]"
		}
		here := appendPath(at, idx)
		if isComp {
			// The body is the item: `{name: c}` or an embedded `{c}`.
			w.value(itemOf(c.Value), here, joinCond(cond, c.Clauses), true)
			continue
		}
		w.value(el, here, cond, true)
	}
}

// itemOf is the item a list comprehension produces: its body, or what the body
// embeds when that is all it holds.
func itemOf(body ast.Expr) ast.Expr {
	if st, ok := body.(*ast.StructLit); ok && len(st.Elts) == 1 {
		if emb, ok := st.Elts[0].(*ast.EmbedDecl); ok {
			return emb.Expr
		}
	}
	return body
}

func appendPath(at []string, elem string) []string {
	return append(append(make([]string, 0, len(at)+1), at...), elem)
}

// joinCond adds a comprehension's clauses, as written and on one line, to the
// ones already guarding it.
func joinCond(outer string, clauses []ast.Clause) string {
	parts := make([]string, 0, len(clauses)+1)
	if outer != "" {
		parts = append(parts, outer)
	}
	for _, c := range clauses {
		parts = append(parts, clauseText(c))
	}
	return strings.Join(parts, " ")
}

// clauseText writes one clause as the author did, on one line. format.Node
// does not take a clause on its own, so its expressions are formatted instead.
func clauseText(c ast.Clause) string {
	switch c := c.(type) {
	case *ast.IfClause:
		return "if " + exprText(c.Condition)
	case *ast.ForClause:
		vars := exprText(c.Value)
		if c.Key != nil {
			vars = exprText(c.Key) + ", " + vars
		}
		return "for " + vars + " in " + exprText(c.Source)
	case *ast.LetClause:
		return "let " + exprText(c.Ident) + " = " + exprText(c.Expr)
	default:
		return "a condition"
	}
}

// steeredByParameters reports whether a condition, as clauseText spells it,
// reads the parent's parameters and nothing else, so a child can steer it
// through $super. Names a for clause or a let declares are its own, and a
// built-in type is not a reference.
func steeredByParameters(cond string) bool {
	file, err := parser.ParseFile("cond.cue", "x: {"+cond+" {}}")
	if err != nil {
		return false
	}
	declared := map[string]bool{}
	ast.Walk(file, func(n ast.Node) bool {
		switch c := n.(type) {
		case *ast.ForClause:
			if c.Key != nil {
				declared[c.Key.Name] = true
			}
			if c.Value != nil {
				declared[c.Value.Name] = true
			}
		case *ast.LetClause:
			declared[c.Ident.Name] = true
		}
		return true
	}, nil)
	sawParameter, other := false, false
	var roots func(n ast.Node) bool
	roots = func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.SelectorExpr:
			// Only the start of a selector chain is a reference; the rest are
			// field names.
			ast.Walk(v.X, roots, nil)
			return false
		case *ast.Field:
			ast.Walk(v.Value, roots, nil)
			return false
		case *ast.Ident:
			switch {
			case v.Name == ParameterField:
				sawParameter = true
			case declared[v.Name], builtinType[v.Name], v.Name == "x":
			default:
				other = true
			}
		}
		return true
	}
	ast.Walk(file, roots, nil)
	return sawParameter && !other
}

func exprText(e ast.Expr) string {
	b, err := format.Node(e)
	if err != nil {
		return "?"
	}
	return strings.Join(strings.Fields(string(b)), " ")
}

// leafPath spells a child leaf's path in the elements guardedField uses.
func leafPath(field string, sels []cue.Selector) []string {
	out := make([]string, 0, len(sels)+1)
	out = append(out, field)
	for _, sel := range sels {
		switch {
		case sel.Type() == cue.IndexLabel:
			out = append(out, "["+sel.String()+"]")
		case sel.IsString():
			out = append(out, sel.Unquoted())
		default:
			out = append(out, sel.String())
		}
	}
	return out
}

// refPath reads a reference such as `parameter.annotations` or
// `parameter["cpu"]` as a path. ok is false for anything else, and for a hidden
// field or a definition, which a path from outside the file cannot name.
func refPath(e ast.Expr) ([]cue.Selector, bool) {
	switch v := e.(type) {
	case *ast.Ident:
		if strings.HasPrefix(v.Name, "_") || strings.HasPrefix(v.Name, "#") {
			return nil, false
		}
		return []cue.Selector{cue.Str(v.Name)}, true
	case *ast.SelectorExpr:
		head, ok := refPath(v.X)
		name := labelName(v.Sel)
		if !ok || name == "" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, "#") {
			return nil, false
		}
		return append(head, cue.Str(name)), true
	case *ast.IndexExpr:
		head, ok := refPath(v.X)
		lit, isLit := v.Index.(*ast.BasicLit)
		if !ok || !isLit || lit.Kind != token.STRING {
			return nil, false
		}
		return append(head, cue.Str(labelName(lit))), true
	case *ast.ParenExpr:
		return refPath(v.X)
	}
	return nil, false
}

// lookupRef follows a path one element at a time, through an optional field
// too, such as `annotations?`, which a plain lookup does not return.
func lookupRef(v cue.Value, sels []cue.Selector) cue.Value {
	for _, sel := range sels {
		next := v.LookupPath(cue.MakePath(sel))
		if !next.Exists() {
			next = v.LookupPath(cue.MakePath(sel.Optional()))
		}
		v = next
		if !v.Exists() {
			return v
		}
	}
	return v
}

// mergesByKey drops the guarded fields whose value can be a struct, such as
// `annotations: parameter.annotations` with `annotations?: [string]: string`.
// A struct merges with what a child writes key by key, so writing below it is
// an addition, and a key both set is refused at render by checkOverride. The
// kind is read from anc compiled with nothing bound, as admission compiles it.
// A value that is not a plain reference, or that does not resolve, is kept.
func mergesByKey(ctx context.Context, anc Level, guarded []guardedField, compile CompileFunc) []guardedField {
	var unbound cue.Value
	compiled, usable := false, false
	kept := guarded[:0:0]
	for _, g := range guarded {
		sels, ok := refPath(g.value)
		if ok && compile != nil {
			if !compiled {
				// `$super` is opened as fixedAmong does, and `context` too, which
				// the render compiler does not bind here.
				var compileErr error
				unbound, compileErr = compile(ctx, anc.Template+"\n"+SuperField+": _\ncontext: _\n")
				// A failed or partial compile says nothing reliable about a
				// kind, so every guarded field stays protected.
				usable = compileErr == nil && unbound.Exists() && unbound.Err() == nil
				compiled = true
			}
			// Only a value that can be nothing but a struct merges key by key; a
			// scalar alternative such as `string | [string]: string` would clash.
			if usable {
				if ref := lookupRef(unbound, sels); ref.Exists() && ref.IncompleteKind() == cue.StructKind {
					continue
				}
			}
		}
		kept = append(kept, g)
	}
	return kept
}

// checkGuarded refuses a field of own at or below one that anc writes only
// under a comprehension, as a scalar or a list. An output listed in cut was
// left out on the way up and is the level's own, so it is not compared.
func checkGuarded(ctx context.Context, at string, own cue.Value, anc Level, s Surface, cut map[string]bool, compile CompileFunc) error {
	guarded := guardedFields(anc, s)
	if len(guarded) == 0 {
		return nil
	}
	guarded = mergesByKey(ctx, anc, guarded, compile)
	if len(guarded) == 0 {
		return nil
	}
	for _, field := range s.Fields {
		cv := own.LookupPath(cue.ParsePath(field))
		if !cv.Exists() {
			continue
		}
		var hit, cond string
		walkLeaves(cv, nil, func(sels []cue.Selector, _ cue.Value) bool {
			if len(sels) > 0 && sels[0].IsString() && cut[field+"."+sels[0].Unquoted()] {
				return true
			}
			path := leafPath(field, sels)
			for _, g := range guarded {
				if g.covers(path) {
					hit, cond = pathString(field, sels), g.cond
					return false
				}
			}
			return true
		})
		if hit != "" {
			// Only a condition on the parent's parameters can be steered from a
			// child; one on context or anything else cannot.
			fix := fmt.Sprintf("Pass a value to %s through `%s: properties` instead", anc.Name, SuperField)
			if !steeredByParameters(cond) {
				fix = fmt.Sprintf("To use another value, write a definition that does not extend %s", anc.Name)
			}
			return fmt.Errorf("%s %s sets %s, which %s sets only when `%s`; an Application that meets that condition "+
				"would be refused. %s",
				s.Kind, at, hit, anc.Name, cond, fix)
		}
	}
	return nil
}

// checkGuardedChain runs checkGuarded against every ancestor, nearest first,
// adding each one's own drops to the cuts as it goes up. cut is not changed.
func checkGuardedChain(ctx context.Context, at string, own cue.Value, ancestors []Level, s Surface, cut map[string]bool, compile CompileFunc) error {
	seen := make(map[string]bool, len(cut))
	for k, v := range cut {
		seen[k] = v
	}
	for _, anc := range ancestors {
		if err := checkGuarded(ctx, at, own, anc, s, seen, compile); err != nil {
			return err
		}
		ad, err := parseDirectives(anc)
		if err != nil {
			return err
		}
		cutsOf(ad, seen)
	}
	return nil
}
