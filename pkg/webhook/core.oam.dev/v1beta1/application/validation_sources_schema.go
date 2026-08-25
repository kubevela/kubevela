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

package application

import (
	"strconv"

	"cuelang.org/go/cue"
)

// cueStruct wraps a struct cue.Value with dotted-path lookup helpers. It backs
// both the source schema (output contract) and the source/target parameter
// (input contract) validators; the path/type helpers are identical for both.
type cueStruct struct {
	root cue.Value
}

// lookup walks a path through the struct, resolving optional fields the same
// way sourceSchemaValidator does.
//
// The path arrives as segments rather than dotted text because a property key
// may itself contain a dot - `headers: {"content.type": ...}`, or any
// Kubernetes-style label - and splitting the rendered path would read one key
// as two.
func (c *cueStruct) lookup(segs []string) (cue.Value, bool) {
	cur := c.root
	for _, seg := range segs {
		if seg == "" {
			return cur, false
		}
		if idx, err := strconv.Atoi(seg); err == nil {
			next, ok := listElementAt(cur, idx)
			if !ok {
				return next, false
			}
			cur = next
			continue
		}
		next := cur.LookupPath(cue.MakePath(cue.Str(seg)))
		if !next.Exists() {
			if opt, ok := lookupOptionalField(cur, seg); ok {
				cur = opt
				continue
			}
			// An open map - headers?: [string]: string - declares no concrete
			// field at any key, only a value type, so match the pattern
			// constraint rather than looking for the key itself.
			if pattern := cur.LookupPath(cue.MakePath(cue.AnyString)); pattern.Exists() {
				cur = pattern
				continue
			}
			return next, false
		}
		cur = next
	}
	return cur, true
}

// valueAt returns the declared CUE value at path, for checks that need more than
// a kind - comparing a collection's element type, for one.
func (c *cueStruct) valueAt(segs []string) (cue.Value, bool) {
	v, ok := c.lookup(segs)
	if !ok || !v.Exists() {
		return cue.Value{}, false
	}
	return v, true
}

// listElementAt resolves one index of a list-valued schema to the type its
// elements must satisfy.
//
// Properties are flattened to dotted leaves, so paths: ["a","b"] arrives as
// paths.0 and paths.1 and each index must resolve. Three shapes have to work:
//
//	[string, string]              a concrete element per index
//	[...string]                   an element type only
//	[...string] | *["app.yaml"]   a disjunction, where neither resolves directly
//
// The disjunction is decomposed with Expr and each branch tried. A branch can be
// semantically Equal to the value it came from while behaving differently -
// indexing the disjunction resolves against its default, indexing the branch
// against the whole list - so this recurses on a depth bound rather than on
// whether the branch differs.
//
// An index outside every branch is refused, which keeps a closed list closed.
func listElementAt(v cue.Value, idx int) (cue.Value, bool) {
	return listElementAtDepth(v, idx, 4)
}

func listElementAtDepth(v cue.Value, idx, depth int) (cue.Value, bool) {
	if elem := v.LookupPath(cue.MakePath(cue.Index(idx))); elem.Exists() {
		return elem, true
	}
	// An open list has no concrete element at any index, only an element type.
	if elem := v.LookupPath(cue.MakePath(cue.AnyIndex)); elem.Exists() {
		return elem, true
	}
	if depth <= 0 {
		return cue.Value{}, false
	}
	// A disjunction hides the list behind an operator, so neither lookup above
	// reaches it.
	if _, branches := v.Expr(); len(branches) > 0 {
		for _, branch := range branches {
			if elem, ok := listElementAtDepth(branch, idx, depth-1); ok {
				return elem, true
			}
		}
	}
	return cue.Value{}, false
}

// kindAt returns the declared CUE kind at path (e.g. StringKind, IntKind,
// StructKind). Returns (BottomKind, false) if the path does not resolve.
func (c *cueStruct) kindAt(segs []string) (cue.Kind, bool) {
	v, ok := c.lookup(segs)
	if !ok || !v.Exists() {
		return cue.BottomKind, false
	}
	return v.IncompleteKind(), true
}

// requiredAt reports whether path names a field that the struct declares AND
// requires: present, not optional, and with no default to fall back on.
func (c *cueStruct) requiredAt(segs []string) bool {
	if len(segs) == 0 || segs[len(segs)-1] == "" {
		return false
	}
	leaf := segs[len(segs)-1]
	if _, err := strconv.Atoi(leaf); err == nil {
		return false // array index: not a named required field
	}
	parent := c.root
	if len(segs) > 1 {
		p, ok := c.lookup(segs[:len(segs)-1])
		if !ok {
			return false
		}
		parent = p
	}
	iter, err := parent.Fields(cue.Optional(true), cue.Definitions(false))
	if err != nil {
		return false
	}
	for iter.Next() {
		sel := iter.Selector()
		if !sel.IsString() || sel.Unquoted() != leaf {
			continue
		}
		if iter.IsOptional() {
			return false
		}
		if _, hasDefault := iter.Value().Default(); hasDefault {
			return false // defaulted -> not required
		}
		return true
	}
	return false
}

// kindName renders a CUE kind for user-facing error messages.
func kindName(k cue.Kind) string {
	//nolint:exhaustive // the kinds a user can write; anything else falls through to the default below
	switch k {
	case cue.StringKind:
		return "string"
	case cue.IntKind:
		return "int"
	case cue.NumberKind, cue.FloatKind:
		return "number"
	case cue.BoolKind:
		return "bool"
	case cue.StructKind:
		return "object"
	case cue.ListKind:
		return "list"
	case cue.NullKind:
		return "null"
	}
	return k.String()
}

// kindsCompatible reports whether a value of kind src can satisfy a target of
// kind dst.
//
// Compatibility is by kind intersection, permissive enough to avoid false
// positives from value-level constraints - enums, bounds - while still catching
// a genuine mismatch such as string into int.
//
// Numbers are compatible in both directions, and the float-into-int direction is
// deliberate rather than an oversight. CEL types arithmetic as double even when
// every value involved is integral, so `$(source.cfg.port / 2)` is a double
// feeding an int parameter, and refusing it would reject an expression that
// resolves perfectly well. Whether the value really is integral is not knowable
// here - this runs before anything is fetched - so the check that a fractional
// value cannot reach an int parameter is CUE's, when the resolved value is
// unified against the schema at render.
//
// An unknown kind on either side is accepted. This check exists to catch a
// mismatch it can prove, and refusing what it cannot type would make an
// unparseable definition look like a broken Application.
func kindsCompatible(src, dst cue.Kind) bool {
	if src == cue.BottomKind || dst == cue.BottomKind {
		return true // unknown on either side: do not block
	}
	if src&dst != 0 {
		return true
	}
	// int is a subset of number/float.
	if src == cue.IntKind && dst&(cue.NumberKind|cue.FloatKind) != 0 {
		return true
	}
	if dst == cue.IntKind && src&(cue.NumberKind|cue.FloatKind) != 0 {
		return true
	}
	return false
}
