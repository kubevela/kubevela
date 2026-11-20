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
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
	"github.com/getkin/kin-openapi/openapi3"
)

// Kind is the shape of a parameter field.
type Kind string

// Kinds a parameter field can take.
const (
	KindString  Kind = "string"
	KindInt     Kind = "integer"
	KindNumber  Kind = "number"
	KindBool    Kind = "boolean"
	KindBytes   Kind = "bytes"
	KindObject  Kind = "object"
	KindMap     Kind = "map"
	KindArray   Kind = "array"
	KindOneOf   Kind = "oneOf"
	KindAny     Kind = "any"
	KindUnknown Kind = ""
)

// maxDepth bounds the walk, whatever the template's shape.
const maxDepth = 32

// Field is one parameter, as read from the compiled template.
type Field struct {
	Name        string
	Kind        Kind
	Description string
	Immutable   bool
	// Optional is true for a field that need not be supplied: marked `?`, or
	// carrying a default.
	Optional bool
	Nullable bool
	// HasDefault distinguishes a null default from none.
	HasDefault bool
	Default    any
	Enum       []any

	Min, Max                   *float64
	ExclusiveMin, ExclusiveMax bool
	MinLength, MaxLength       *uint64
	MinItems, MaxItems         *uint64
	MinProperties              *uint64
	MultipleOf                 *float64
	// Patterns must all match; NotPatterns must none.
	Patterns    []string
	NotPatterns []string
	NotEqual    []any
	UniqueItems bool

	// Fields are an object's fields, in declaration order.
	Fields []*Field
	// Items is an array's element.
	Items *Field
	// Values is a map's value.
	Values *Field
	// Variants are the alternatives of a disjunction that is not an enum.
	Variants []*Field
	// Open marks an object that accepts fields beyond those it declares.
	Open bool
	// Recursive marks an object cut short where its type refers to itself.
	Recursive bool
	// Conditions are set on a field that exists only for some values of
	// other fields; all of them must hold.
	Conditions []Condition
	// pos is where the field is declared, which orders a form.
	pos token.Pos
	// clause is the marker of the `if` body that declares the field.
	clause string
	// UI are the field's `+ui:` hints.
	UI UIHints
	// Discriminators name the siblings whose values decide which conditional
	// fields of this object exist.
	Discriminators []string
}

// BuildField reads a value of a compiled template, such as its parameter. src
// is the template's source, used only to find which fields an `if` reads.
func BuildField(v cue.Value, src string) *Field {
	w := &walker{root: v}
	w.condNames, w.clauses = scanSource(src)
	return w.field("", v, cue.Path{}, nil, 0)
}

type walker struct {
	root      cue.Value
	condNames map[string]bool
	// clauses are the `if` bodies of the template, keyed by their marker
	// (see InstrumentClauses) or, unmarked, by their position.
	clauses map[string]clause
	// scopes are the discriminators of the objects enclosing the one being
	// walked, innermost last.
	scopes []scope
}

func (w *walker) field(name string, v cue.Value, path cue.Path, refs []string, depth int) *Field {
	f := &Field{Name: name}
	if depth > maxDepth {
		f.Kind, f.Recursive = KindObject, true
		return f
	}
	f.apply(docOf(v))

	if root, ref := v.ReferencePath(); root.Exists() && ref.String() != "" {
		r := ref.String()
		for _, seen := range refs {
			if seen == r {
				f.Kind, f.Recursive = KindObject, true
				return f
			}
		}
		refs = append(append([]string(nil), refs...), r)
	}

	op, args := unwrap(v)
	if d, ok := v.Default(); ok && d.IsConcrete() && d.Validate(cue.Concrete(true)) == nil {
		var x any
		if err := d.Decode(&x); err == nil && !emptyOpen(x, op) {
			// A null default makes the field optional but is not shown.
			f.Default = x
			f.HasDefault = true
		}
	}

	kind, shape := effectiveKind(v)
	if kind&cue.NullKind != 0 && kind != cue.NullKind && kind != cue.TopKind {
		f.Nullable = true
		kind &^= cue.NullKind
	}

	if op == cue.OrOp {
		w.disjunction(f, v, args, path, refs, depth)
		return f
	}

	switch kind {
	case cue.StringKind:
		f.Kind = KindString
	case cue.IntKind:
		f.Kind = KindInt
	case cue.FloatKind, cue.NumberKind:
		f.Kind = KindNumber
	case cue.BoolKind:
		f.Kind = KindBool
	case cue.BytesKind:
		f.Kind = KindBytes
	case cue.StructKind:
		w.object(f, shape, path, refs, depth)
	case cue.ListKind:
		f.Kind = KindArray
		if elem, ok := listElem(shape); ok {
			f.Items = w.field("", elem, appendPath(path, cue.AnyIndex), refs, depth+1)
		} else if it, err := shape.List(); err == nil && it.Next() {
			// A closed list reads its element's shape from the first entry,
			// but not that entry's value.
			f.Items = w.field("", it.Value(), path, refs, depth+1)
			f.Items.Enum, f.Items.Default, f.Items.HasDefault = nil, nil, false
		}
	case cue.TopKind:
		f.Kind = KindAny
	default:
		if kind.IsAnyOf(cue.StringKind | cue.NumberKind | cue.BoolKind) {
			f.Kind = KindAny
		} else {
			f.Kind = KindUnknown
		}
	}
	if v.IsConcrete() && kind&(cue.StructKind|cue.ListKind) == 0 {
		var x any
		if err := v.Decode(&x); err == nil {
			f.Enum = []any{x}
		}
	}
	constraints(f, v)
	return f
}

// disjunction reads `a | b | c`: an enum when every branch is concrete,
// otherwise variants.
func (w *walker) disjunction(f *Field, v cue.Value, args []cue.Value, path cue.Path, refs []string, depth int) {
	var enum []any
	var kinds []cue.Kind
	allConcrete := true
	for _, a := range args {
		if a.IncompleteKind() == cue.NullKind {
			f.Nullable = true
			continue
		}
		if !a.IsConcrete() || a.IncompleteKind()&(cue.StructKind|cue.ListKind) != 0 {
			allConcrete = false
			break
		}
		var x any
		if err := a.Decode(&x); err != nil {
			allConcrete = false
			break
		}
		if !containsValue(enum, x) {
			enum = append(enum, x)
		}
		kinds = append(kinds, a.IncompleteKind())
	}
	if allConcrete && len(enum) > 0 {
		f.Enum = enum
		f.Kind = kindOf(kinds)
		return
	}

	// A `_` arm, such as a default read from the open `context`, says
	// nothing about the shape.
	var branches []cue.Value
	nullable := false
	for _, a := range args {
		switch a.IncompleteKind() {
		case cue.NullKind:
			nullable = true
		case cue.TopKind:
		default:
			branches = append(branches, a)
		}
	}
	if len(branches) == 0 {
		f.Kind, f.Nullable = KindAny, nullable
		return
	}
	if k, ok := sameScalarKind(branches); ok && len(branches) > 1 {
		// `"go" | "java" | string` is a string: the literals are
		// suggestions.
		f.Kind, f.Nullable = k, nullable
		if pattern, ok := alternation(branches); ok {
			f.Patterns = []string{pattern}
			return
		}
		for _, b := range branches {
			if s, err := b.String(); err == nil && b.IsConcrete() && !containsString(f.UI.Suggest, s) {
				f.UI.Suggest = append(f.UI.Suggest, s)
			}
		}
		return
	}
	if len(branches) == 1 {
		inner := w.field(f.Name, branches[0], path, refs, depth)
		inner.Default, inner.HasDefault, inner.Nullable = f.Default, f.HasDefault, nullable
		if inner.Description == "" {
			inner.Description = f.Description
		}
		inner.Immutable = inner.Immutable || f.Immutable
		if inner.UI.empty() {
			inner.UI = f.UI
		}
		*f = *inner
		return
	}

	// A list with a default is `*[...] | [...T]`: the schema is the open list.
	if v.IncompleteKind() == cue.ListKind {
		for _, b := range branches {
			if b.Len().IsConcrete() {
				// A closed list is the default's value, not the element type.
				continue
			}
			if elem := b.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
				f.Kind = KindArray
				f.Items = w.field("", elem, appendPath(path, cue.AnyIndex), refs, depth+1)
				return
			}
		}
	}

	f.Kind = KindOneOf
	for _, b := range branches {
		f.Variants = append(f.Variants, w.field("", b, path, refs, depth+1))
	}
}

func (w *walker) fields(v cue.Value, path cue.Path, refs []string, depth int) []*Field {
	it, err := v.Fields(cue.Optional(true))
	if err != nil {
		return nil
	}
	var out []*Field
	for it.Next() {
		sel := it.Selector()
		name := labelName(sel)
		child := w.field(name, it.Value(), appendPath(path, sel), refs, depth+1)
		child.pos = it.Value().Pos()
		w.placeInClause(child, v)
		if sel.ConstraintType() == cue.OptionalConstraint || child.HasDefault {
			child.Optional = true
		}
		out = append(out, child)
	}
	return out
}

// constraints reads bounds, patterns and validators from a conjunction.
func constraints(f *Field, v cue.Value) {
	for _, p := range constraintParts(v, 0) {
		pop, pargs := unwrap(p)
		switch pop {
		case cue.GreaterThanEqualOp, cue.GreaterThanOp:
			if n, ok := number(pargs); ok {
				if f.Min == nil || n > *f.Min {
					f.Min, f.ExclusiveMin = &n, pop == cue.GreaterThanOp
				}
			}
		case cue.LessThanEqualOp, cue.LessThanOp:
			if n, ok := number(pargs); ok {
				if f.Max == nil || n < *f.Max {
					f.Max, f.ExclusiveMax = &n, pop == cue.LessThanOp
				}
			}
		case cue.RegexMatchOp:
			if s, ok := str(pargs); ok && !containsString(f.Patterns, s) {
				f.Patterns = append(f.Patterns, s)
			}
		case cue.NotRegexMatchOp:
			if s, ok := str(pargs); ok && !containsString(f.NotPatterns, s) {
				f.NotPatterns = append(f.NotPatterns, s)
			}
		case cue.NotEqualOp:
			if len(pargs) == 1 {
				var x any
				if pargs[0].Decode(&x) == nil {
					f.NotEqual = append(f.NotEqual, x)
				}
			}
		case cue.CallOp:
			call(f, pargs)
		default:
			// Other operators constrain nothing a schema shows.
		}
	}
	// int32 and friends arrive as bounds; their endpoints are the kind's own
	// range, not a constraint worth showing, but a user bound tightens either.
	if f.Kind == KindInt && f.Min != nil && f.Max != nil {
		if *f.Min <= -2147483648 {
			f.Min = nil
		}
		if *f.Max >= 2147483647 {
			f.Max = nil
		}
	}
}

func call(f *Field, args []cue.Value) {
	if len(args) == 0 {
		return
	}
	name := funcName(args[0])
	var n *uint64
	if len(args) > 1 {
		if i, err := args[1].Uint64(); err == nil {
			n = &i
		}
	}
	switch name {
	case "strings.MinRunes":
		f.MinLength = n
	case "strings.MaxRunes":
		f.MaxLength = n
	case "list.MinItems":
		f.MinItems = n
	case "list.MaxItems":
		f.MaxItems = n
	case "list.UniqueItems":
		f.UniqueItems = true
	case "struct.MinFields":
		f.MinProperties = n
	case "math.MultipleOf":
		if len(args) > 1 {
			if x, err := args[1].Float64(); err == nil {
				f.MultipleOf = &x
			}
		}
	}
}

func funcName(fn cue.Value) string {
	if op, args := fn.Expr(); op == cue.SelectorOp && len(args) == 2 {
		if sel, err := args[1].String(); err == nil {
			return pkgName(args[0]) + "." + sel
		}
	}
	return fmt.Sprint(fn)
}

// pkgName names a builtin package from the functions it holds, since the
// package value itself has no name.
func pkgName(pkg cue.Value) string {
	for _, candidate := range []struct{ field, name string }{
		{"MinRunes", "strings"}, {"MinItems", "list"}, {"MinFields", "struct"}, {"MultipleOf", "math"},
	} {
		if pkg.LookupPath(cue.ParsePath(candidate.field)).Exists() {
			return candidate.name
		}
	}
	return ""
}

// constraintParts splits a value into the constraints it is the conjunction
// of, following references, so `#Name & strings.MaxRunes(5)` yields #Name's
// pattern as well as the length.
func constraintParts(v cue.Value, depth int) []cue.Value {
	if depth > maxDepth {
		return nil
	}
	op, args := unwrap(v)
	switch op {
	case cue.SelectorOp:
		if d := cue.Dereference(v); d != v {
			return constraintParts(d, depth+1)
		}
	case cue.AndOp:
		var out []cue.Value
		for _, a := range args {
			out = append(out, constraintParts(a, depth+1)...)
		}
		return out
	default:
		// Other operators are a single constraint.
	}
	return []cue.Value{v}
}

// unwrap strips the NoOp layers Expr puts around a value.
func unwrap(v cue.Value) (cue.Op, []cue.Value) {
	op, args := v.Expr()
	for i := 0; i < maxDepth && op == cue.NoOp && len(args) == 1; i++ {
		nop, nargs := args[0].Expr()
		if nop == cue.NoOp && len(nargs) == 1 && reflect.DeepEqual(nargs, args) {
			break
		}
		op, args = nop, nargs
	}
	return op, args
}

func flattenAnd(args []cue.Value) []cue.Value {
	var out []cue.Value
	for _, a := range args {
		op, inner := unwrap(a)
		if op == cue.AndOp {
			out = append(out, flattenAnd(inner)...)
			continue
		}
		out = append(out, a)
	}
	return out
}

func number(args []cue.Value) (float64, bool) {
	if len(args) != 1 {
		return 0, false
	}
	n, err := args[0].Float64()
	return n, err == nil
}

func str(args []cue.Value) (string, bool) {
	if len(args) != 1 {
		return "", false
	}
	s, err := args[0].String()
	return s, err == nil
}

func kindOf(kinds []cue.Kind) Kind {
	var k cue.Kind
	for _, x := range kinds {
		k |= x
	}
	switch k {
	case cue.StringKind:
		return KindString
	case cue.IntKind:
		return KindInt
	case cue.FloatKind, cue.NumberKind:
		return KindNumber
	case cue.BoolKind:
		return KindBool
	default:
		// Other kinds have no scalar kind.
	}
	return KindAny
}

func containsValue(list []any, x any) bool {
	for _, y := range list {
		if reflect.DeepEqual(x, y) {
			return true
		}
	}
	return false
}

func pathJoin(p cue.Path, name string) cue.Path {
	return appendPath(p, cue.Str(name))
}

func appendPath(p cue.Path, sel cue.Selector) cue.Path {
	return cue.MakePath(append(append([]cue.Selector(nil), p.Selectors()...), sel)...)
}

func labelName(sel cue.Selector) string {
	if sel.LabelType() == cue.StringLabel {
		return sel.Unquoted()
	}
	return strings.TrimRight(sel.String(), "?!")
}

func literalString(lit *ast.BasicLit) string {
	return strings.Trim(lit.Value, `"`)
}

// effectiveKind is a value's kind, and the part of it that carries the shape.
// A conjunction with a validator (`[...string] & list.MinItems(1)`) and a
// struct holding an unresolved `if` both report bottom, though their shape is
// plain.
func effectiveKind(v cue.Value) (cue.Kind, cue.Value) {
	if k := v.IncompleteKind(); k != cue.BottomKind {
		return k, v
	}
	if op, args := unwrap(v); op == cue.AndOp {
		kind, shape := cue.TopKind, v
		for _, a := range flattenAnd(args) {
			if aop, _ := unwrap(a); aop == cue.CallOp {
				continue
			}
			if k := a.IncompleteKind(); k != cue.BottomKind {
				kind &= k
				if k&(cue.StructKind|cue.ListKind) != 0 {
					shape = a
				}
			}
		}
		if kind != cue.TopKind && kind != cue.BottomKind {
			return kind, shape
		}
	}
	if it, err := v.Fields(cue.Optional(true)); err == nil && it.Next() {
		return cue.StructKind, v
	}
	return cue.BottomKind, v
}

// emptyOpen reports the empty value CUE gives as the default of an open list
// or struct that declares none.
func emptyOpen(x any, op cue.Op) bool {
	if op == cue.OrOp {
		return false
	}
	switch x := x.(type) {
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func sameScalarKind(vs []cue.Value) (Kind, bool) {
	var k cue.Kind
	for i, v := range vs {
		vk := v.IncompleteKind()
		if i > 0 && vk != k {
			return KindUnknown, false
		}
		k = vk
	}
	switch k {
	case cue.StringKind, cue.IntKind, cue.FloatKind, cue.NumberKind, cue.BoolKind:
		return kindOf([]cue.Kind{k}), true
	default:
		// Other kinds are not scalars.
	}
	return KindUnknown, false
}

// listElem is an open list's element type. A list with a default resolves
// to the default when looked into, so its open branch is preferred. Only a
// disjunction's: the parts of any other value leave out what was filled into
// it.
func listElem(v cue.Value) (cue.Value, bool) {
	op, args := unwrap(v)
	if op == cue.AndOp {
		// A value filled into a list with a default: the element is the open
		// branch's and the fill's together.
		var elem cue.Value
		found := false
		for _, part := range flattenAnd(args) {
			e, ok := listElem(part)
			if !ok {
				continue
			}
			if found {
				elem = elem.Unify(e)
			} else {
				elem, found = e, true
			}
		}
		if found {
			return elem, true
		}
	}
	if op == cue.OrOp {
		for _, a := range args {
			if a.Len().IsConcrete() {
				continue
			}
			if e := a.LookupPath(cue.MakePath(cue.AnyIndex)); e.Exists() {
				return e, true
			}
		}
	}
	if e := v.LookupPath(cue.MakePath(cue.AnyIndex)); e.Exists() {
		return e, true
	}
	for _, a := range args {
		if e := a.LookupPath(cue.MakePath(cue.AnyIndex)); e.Exists() {
			return e, true
		}
	}
	return cue.Value{}, false
}

// alternation is the one pattern a disjunction of patterns and string literals
// matches: `=~"^a" | =~"^b" | "c"` is `(?:^a)|(?:^b)|(?:^c$)`. A branch that is
// neither, such as a plain string, means anything matches, so there is none.
func alternation(branches []cue.Value) (string, bool) {
	var alts []string
	sawPattern := false
	for _, b := range branches {
		if s, err := b.String(); err == nil && b.IsConcrete() {
			alts = append(alts, "(?:^"+regexp.QuoteMeta(s)+"$)")
			continue
		}
		pattern := ""
		for _, p := range constraintParts(b, 0) {
			if op, args := unwrap(p); op == cue.RegexMatchOp {
				if s, ok := str(args); ok && pattern == "" {
					pattern = s
					continue
				}
				return "", false
			}
		}
		if pattern == "" {
			return "", false
		}
		sawPattern = true
		alts = append(alts, "(?:"+pattern+")")
	}
	if !sawPattern {
		return "", false
	}
	return strings.Join(alts, "|"), true
}

// splitPatterns renders patterns that must all match: the first as a schema's
// pattern and each further one as an allOf entry, since a schema holds one.
func splitPatterns(patterns []string) (string, openapi3.SchemaRefs) {
	if len(patterns) == 0 {
		return "", nil
	}
	var more openapi3.SchemaRefs
	for _, p := range patterns[1:] {
		more = append(more, openapi3.NewSchemaRef("", &openapi3.Schema{Pattern: p}))
	}
	return patterns[0], more
}

// anyPattern is the one pattern that matches where any of patterns does.
func anyPattern(patterns []string) string {
	if len(patterns) == 1 {
		return patterns[0]
	}
	alts := make([]string, len(patterns))
	for i, p := range patterns {
		alts[i] = "(?:" + p + ")"
	}
	return strings.Join(alts, "|")
}

// allPatterns is the one pattern that matches where every one of patterns
// does. It is a run of lookaheads, which VelaUX's JavaScript RegExp supports
// and Go's regexp does not, so it is only for the UI schema.
func allPatterns(patterns []string) string {
	if len(patterns) <= 1 {
		return strings.Join(patterns, "")
	}
	var b strings.Builder
	b.WriteString("^")
	for _, p := range patterns {
		b.WriteString(`(?=[\s\S]*(?:` + p + `))`)
	}
	return b.String()
}
