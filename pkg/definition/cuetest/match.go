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

package cuetest

import (
	"fmt"
	"slices"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

// Match reports every way actual fails to be an instance of expect, one line
// per path. The rules are CUE subsumption: a regular field must be present and
// match, an optional field of bottom (`f?: _|_`) must be absent, and a list
// without `...` fixes the length. Attributes on an expected field refine it:
// @exact() admits no fields beyond those given, @not() passes unless the field
// is present and matches, and @contains() matches a list's elements in any
// position, each against a different element; with @not() too, no listed
// element may be present. An expected disjunction matches when any of its
// alternatives does, its default no different from the rest. Match walks the
// expectation itself because
// cue.Value.Subsume names the wrong cause for a mismatched value.
//
// Exactness is an attribute, not CUE closedness: the vela/test definitions
// close every struct unified into them from another value, such as a shared
// _base, so closedness does not say what the author wrote.
func Match(expect, actual cue.Value) []string {
	return match(expect, actual, nil)
}

// match is Match, passing over expected fields at the paths skip reports.
func match(expect, actual cue.Value, skip func(path []cue.Selector) bool) []string {
	m := &matcher{skip: skip}
	m.walk(nil, expect, actual, mods{})
	return m.failures
}

// isCheckCall is a check's call: what it expects is its returns.
func isCheckCall(path []cue.Selector) bool {
	return len(path) == 3 && path[0].String() == "checks" && path[2].String() == "call"
}

type matcher struct {
	failures []string
	skip     func(path []cue.Selector) bool
}

func (m *matcher) fail(path []cue.Selector, format string, args ...any) {
	at := "(root)"
	if len(path) > 0 {
		at = cue.MakePath(path...).String()
	}
	m.failures = append(m.failures, at+": "+fmt.Sprintf(format, args...))
}

// mods are the attributes refining an expected field.
type mods struct {
	exact, not, contains bool
}

func fieldMods(v cue.Value) mods {
	has := func(name string) bool {
		a := v.Attribute(name)
		return a.Err() == nil
	}
	return mods{exact: has("exact"), not: has("not"), contains: has("contains")}
}

// matches reports whether actual matches expect, without recording failures.
func matches(expect, actual cue.Value, md mods) bool {
	sub := &matcher{}
	sub.walk(nil, expect, actual, md)
	return len(sub.failures) == 0
}

func (m *matcher) walk(path []cue.Selector, expect, actual cue.Value, md mods) {
	if alts := disjuncts(expect); alts != nil {
		for _, alt := range alts {
			if matches(alt, actual, md) {
				return
			}
		}
		m.fail(path, "expected %v, got %v", expect, actual)
		return
	}
	switch expect.IncompleteKind() {
	case cue.StructKind:
		m.walkStruct(path, expect, actual, md.exact)
	case cue.ListKind:
		if md.contains {
			m.walkContains(path, expect, actual, md.not)
			return
		}
		m.walkList(path, expect, actual)
	default:
		if err := expect.Subsume(actual, cue.Final()); err != nil {
			m.fail(path, "expected %v, got %v", expect, actual)
		}
	}
}

// disjuncts are the alternatives of an expected disjunction, each built on
// its own; nil when expect is not one. A scalar disjunction without a default
// is left to Subsume.
func disjuncts(expect cue.Value) []cue.Value {
	if _, hasDefault := expect.Default(); !hasDefault {
		switch expect.IncompleteKind() {
		case cue.StructKind:
			if _, err := expect.Fields(cue.Optional(true)); err == nil {
				return nil
			}
		case cue.ListKind:
			if _, err := expect.List(); err == nil {
				return nil
			}
		default:
			return nil
		}
	}
	expr, ok := expect.Syntax().(ast.Expr)
	if !ok {
		return nil
	}
	alts := alternatives(expect.Context(), expr)
	if len(alts) < 2 {
		return nil
	}
	for _, alt := range alts {
		if alt.Err() != nil {
			return nil
		}
	}
	return alts
}

// alternatives spreads expr into the alternatives it is a disjunction of,
// distributing a conjunction over its operands' alternatives and dropping
// those that conflict.
func alternatives(ctx *cue.Context, expr ast.Expr) []cue.Value {
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return alternatives(ctx, x.X)
	case *ast.UnaryExpr:
		if x.Op == token.MUL {
			return alternatives(ctx, x.X)
		}
	case *ast.BinaryExpr:
		switch x.Op {
		case token.OR:
			return append(alternatives(ctx, x.X), alternatives(ctx, x.Y)...)
		case token.AND:
			var out []cue.Value
			for _, l := range alternatives(ctx, x.X) {
				for _, r := range alternatives(ctx, x.Y) {
					if u := l.Unify(r); u.Err() == nil {
						out = append(out, u)
					}
				}
			}
			return out
		default:
		}
	}
	return []cue.Value{ctx.BuildExpr(expr)}
}

func (m *matcher) walkStruct(path []cue.Selector, expect, actual cue.Value, exact bool) {
	if actual.Kind() != cue.StructKind {
		m.fail(path, "expected a struct, got %v", actual)
		return
	}
	fields, err := expect.Fields(cue.Optional(true))
	if err != nil {
		m.fail(path, "invalid expectation: %v", err)
		return
	}
	for fields.Next() {
		sel := fields.Selector()
		at := slices.Concat(path, []cue.Selector{cue.Str(sel.Unquoted())})
		if m.skip != nil && m.skip(at) {
			continue
		}
		want, got := fields.Value(), actual.LookupPath(cue.MakePath(cue.Str(sel.Unquoted())))
		md := fieldMods(want)
		if md.not && !md.contains {
			if got.Exists() && matches(want, got, mods{exact: md.exact}) {
				m.fail(at, "expected not to match %v", want)
			}
			continue
		}
		if sel.ConstraintType() == cue.OptionalConstraint {
			switch {
			case want.Err() != nil && got.Exists():
				m.fail(at, "expected absent, got %v", got)
			case want.Err() == nil && got.Exists():
				m.walk(at, want, got, md)
			}
			continue
		}
		if !got.Exists() {
			if !md.not {
				m.fail(at, "missing, expected %v", want)
			}
			continue
		}
		m.walk(at, want, got, md)
	}
	if !exact {
		return
	}
	extra, err := actual.Fields()
	if err != nil {
		return
	}
	for extra.Next() {
		if !expect.LookupPath(cue.MakePath(extra.Selector())).Exists() {
			m.fail(append(path[:len(path):len(path)], extra.Selector()), "unexpected field, got %v", extra.Value())
		}
	}
}

func (m *matcher) walkList(path []cue.Selector, expect, actual cue.Value) {
	if actual.Kind() != cue.ListKind {
		m.fail(path, "expected a list, got %v", actual)
		return
	}
	want, got := listElems(expect), listElems(actual)
	// A list with `...` has no concrete length, and only fixes a prefix.
	_, closedErr := expect.Len().Int64()
	switch {
	case closedErr == nil && len(got) != len(want):
		m.fail(path, "expected %d elements, got %d", len(want), len(got))
		return
	case closedErr != nil && len(got) < len(want):
		m.fail(path, "expected at least %d elements, got %d", len(want), len(got))
		return
	}
	for i := range want {
		m.walk(append(path[:len(path):len(path)], cue.Index(i)), want[i], got[i], mods{})
	}
}

// walkContains matches each expected element against a different actual
// element, in any position, pairing them as a bipartite matching so an
// element that fits several patterns cannot starve another. With none set,
// no expected element may match any actual one.
func (m *matcher) walkContains(path []cue.Selector, expect, actual cue.Value, none bool) {
	if actual.Kind() != cue.ListKind {
		m.fail(path, "expected a list, got %v", actual)
		return
	}
	want, got := listElems(expect), listElems(actual)
	fits := make([][]bool, len(want))
	for i := range want {
		fits[i] = make([]bool, len(got))
		for j := range got {
			fits[i][j] = matches(want[i], got[j], mods{})
		}
	}
	if none {
		for i := range want {
			for j := range got {
				if fits[i][j] {
					m.fail(path, "expected no element to match %v", want[i])
					break
				}
			}
		}
		return
	}
	pairedWith := make([]int, len(got))
	for j := range pairedWith {
		pairedWith[j] = -1
	}
	var pair func(i int, seen []bool) bool
	pair = func(i int, seen []bool) bool {
		for j := range got {
			if fits[i][j] && !seen[j] {
				seen[j] = true
				if pairedWith[j] < 0 || pair(pairedWith[j], seen) {
					pairedWith[j] = i
					return true
				}
			}
		}
		return false
	}
	for i := range want {
		if !pair(i, make([]bool, len(got))) {
			m.fail(path, "no element matches %v", want[i])
		}
	}
}

func listElems(v cue.Value) []cue.Value {
	var elems []cue.Value
	for it, _ := v.List(); it.Next(); {
		elems = append(elems, it.Value())
	}
	return elems
}
