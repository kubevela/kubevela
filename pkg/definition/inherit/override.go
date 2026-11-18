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
	"bytes"
	"context"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
)

// A child adds to what its parent renders and never changes it. A field the
// parent set, concretely or from a defaulted parameter, is the parent's: the
// child changes it by passing the parent a different parameter. A field the
// parent left open, such as `paused: bool`, is the child's to fill.

// overrideFields are the surface fields a child may only add to. `processing`
// is a trait's own step rather than a result, so it is not compared.
func overrideFields(s Surface) []string {
	out := make([]string, 0, len(s.Fields))
	for _, f := range s.Fields {
		if f != "processing" {
			out = append(out, f)
		}
	}
	return out
}

// cutsOf are the outputs a level leaves out, as `field.key`. Such an output is
// the level's own from then on, so nothing above it is compared with it.
func cutsOf(d directives, into map[string]bool) {
	for field, keys := range d.drops {
		for key := range keys {
			into[field+"."+key] = true
		}
	}
}

// checkOverride refuses a field of own that changes one parent set. parent maps
// each surface field to what the parent produced for it.
func checkOverride(at string, own cue.Value, parent map[string]cue.Value, s Surface, cut map[string]bool, parentName string) error {
	for _, field := range overrideFields(s) {
		pv, ok := parent[field]
		if !ok {
			continue
		}
		cv := own.LookupPath(cue.ParsePath(field))
		if !cv.Exists() {
			continue
		}
		var hit string
		walkLeaves(cv, nil, func(sels []cue.Selector, leaf cue.Value) bool {
			if len(sels) > 0 && sels[0].IsString() && cut[field+"."+sels[0].Unquoted()] {
				return true
			}
			if !changes(pv.LookupPath(cue.MakePath(sels...)), leaf) {
				return true
			}
			hit = pathString(field, sels)
			return false
		})
		if hit != "" {
			return fmt.Errorf("%s %s sets %s, which %s already sets; pass a value to %s through `%s: properties` instead",
				s.Kind, at, hit, parentName, parentName, SuperField)
		}
	}
	return nil
}

// CheckOverrides refuses, when a definition is written, a level whose own
// fields change what one of its ancestors sets. Each ancestor is compiled on
// its own with nothing bound, so a field from a defaulted parameter still
// carries its default, and a field from a required one is left open and judged
// at render instead.
func CheckOverrides(ctx context.Context, at string, self Level, own cue.Value, ancestors []Level, s Surface, compile CompileFunc) error {
	d, err := parseDirectives(self)
	if err != nil {
		return err
	}
	cut := map[string]bool{}
	cutsOf(d, cut)
	for _, anc := range ancestors {
		// A level above the first calls its own parent, so `$super` is opened
		// for it to compile.
		val, _ := compile(ctx, anc.Template+"\n"+SuperField+": _\n")
		if val.Exists() {
			parent := map[string]cue.Value{}
			for _, field := range s.Fields {
				if v := val.LookupPath(cue.ParsePath(field)); v.Exists() {
					parent[field] = v
				}
			}
			if err := checkOverride(at, own, parent, s, cut, anc.Name); err != nil {
				return err
			}
		}
		ad, err := parseDirectives(anc)
		if err != nil {
			return err
		}
		cutsOf(ad, cut)
	}
	return nil
}

// changes reports whether a child value at a path changes the parent's value
// there. A default counts as set: writing even the default pins the field and
// takes it away from the parent's parameter.
func changes(parent, child cue.Value) bool {
	if !parent.Exists() {
		return false
	}
	if _, ok := parent.Default(); ok {
		return true
	}
	if k := parent.IncompleteKind(); k == cue.StructKind || k == cue.ListKind {
		// A struct the child wrote a value over: the merge reports it.
		return false
	}
	if !parent.IsConcrete() {
		return false
	}
	return !sameConcrete(parent, child)
}

// sameConcrete compares two concrete values by their JSON, since they may come
// from different cue.Contexts.
func sameConcrete(a, b cue.Value) bool {
	if !b.IsConcrete() {
		return false
	}
	aj, err := a.MarshalJSON()
	if err != nil {
		return false
	}
	bj, err := b.MarshalJSON()
	if err != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}

// walkLeaves visits every value under v that is not a struct or a list, with
// the selectors leading to it. visit returns false to stop.
func walkLeaves(v cue.Value, sels []cue.Selector, visit func([]cue.Selector, cue.Value) bool) bool {
	switch v.IncompleteKind() {
	case cue.StructKind:
		iter, err := v.Fields()
		if err != nil {
			return visit(sels, v)
		}
		for iter.Next() {
			next := append(append([]cue.Selector{}, sels...), iter.Selector())
			if !walkLeaves(iter.Value(), next, visit) {
				return false
			}
		}
		return true
	case cue.ListKind:
		list, err := v.List()
		if err != nil {
			return visit(sels, v)
		}
		for i := 0; list.Next(); i++ {
			next := append(append([]cue.Selector{}, sels...), cue.Index(i))
			if !walkLeaves(list.Value(), next, visit) {
				return false
			}
		}
		return true
	default:
		return visit(sels, v)
	}
}

// pathString spells a path as a reader would, `containers[0].image`.
func pathString(field string, sels []cue.Selector) string {
	var b strings.Builder
	b.WriteString(field)
	for _, sel := range sels {
		if sel.Type() == cue.IndexLabel {
			fmt.Fprintf(&b, "[%s]", sel.String())
			continue
		}
		b.WriteString(".")
		if sel.IsString() {
			b.WriteString(sel.Unquoted())
		} else {
			b.WriteString(sel.String())
		}
	}
	return b.String()
}
