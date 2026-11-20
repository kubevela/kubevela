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
	"strconv"
	"strings"
	"unicode"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
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

// fixedFunc reports which ancestor writes a value at a path into its template
// as a constant, and that value. A value an ancestor computes from a parameter
// is not fixed: a child changes it through `$super: properties`.
type fixedFunc func(path cue.Path) (owner, value string, isDefault, ok bool)

// checkOverride refuses a field of own that changes one parent set. parent maps
// each surface field to what the parent produced for it.
func checkOverride(at string, own cue.Value, parent map[string]cue.Value, s Surface, cut map[string]bool, parentName string, fixed fixedFunc, ancestors []Level) error {
	for _, field := range overrideFields(s) {
		pv, ok := parent[field]
		if !ok {
			continue
		}
		cv := own.LookupPath(cue.ParsePath(field))
		if !cv.Exists() {
			continue
		}
		var hit, key string
		var hitPath cue.Path
		walkLeaves(cv, nil, func(sels []cue.Selector, leaf cue.Value) bool {
			if len(sels) > 0 && sels[0].IsString() && cut[field+"."+sels[0].Unquoted()] {
				return true
			}
			if !changes(pv.LookupPath(cue.MakePath(sels...)), leaf) {
				return true
			}
			hit = pathString(field, sels)
			hitPath = cue.MakePath(append(cue.ParsePath(field).Selectors(), sels...)...)
			if len(sels) > 0 && sels[0].IsString() {
				key = sels[0].Unquoted()
			}
			return false
		})
		if hit == "" {
			continue
		}
		if fixed != nil {
			if owner, value, isDefault, ok := fixed(hitPath); ok {
				// A default written in the template with no parameter behind it
				// is as fixed for a child as a constant: there is nothing to pass
				// through $super.
				how := fmt.Sprintf("fixes it to %s", value)
				if isDefault {
					how = fmt.Sprintf("sets it to the default %s and has no parameter for it", value)
				}
				// An entry of outputs or patchOutputs can be left out and written
				// again, which is a smaller step than a new definition, unless an
				// ancestor requires it.
				if droppable[field] && key != "" && !requiredBy(ancestors, s, field+"."+key) {
					return fmt.Errorf("%s %s sets %s, but %s %s; to write your own %s, "+
						"leave %s's out with `%s: {%s: {%s: false}}` and build it from `%s.%s.%s`",
						s.Kind, at, hit, owner, how, key, owner, InheritField, field, cueKey(key), SuperField, field, cueKey(key))
				}
				if isDefault {
					return fmt.Errorf("%s %s sets %s, but %s %s; to use another value, write a definition that does not extend %s",
						s.Kind, at, hit, owner, how, owner)
				}
				return fmt.Errorf("%s %s sets %s, but %s %s and a child cannot change it; "+
					"to use another value, write a definition that does not extend %s",
					s.Kind, at, hit, owner, how, owner)
			}
		}
		return fmt.Errorf("%s %s sets %s, which %s already sets; pass a value to %s through `%s: properties` instead",
			s.Kind, at, hit, parentName, parentName, SuperField)
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
	if err := checkDropsFit(self, d, s); err != nil {
		return err
	}
	cut := map[string]bool{}
	cutsOf(d, cut)
	for _, anc := range ancestors {
		// A field the ancestor writes only under a condition is absent from its
		// unbound value below, so it is read from the source first.
		if err := checkGuarded(ctx, at, own, anc, s, cut, compile); err != nil {
			return err
		}
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
			if err := checkOverride(at, own, parent, s, cut, anc.Name, fixedIn(anc.Name, val), ancestors); err != nil {
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

// requiredBy reports whether any ancestor marks an entry, such as
// outputs.ingress, +required, alone or through its whole block.
func requiredBy(ancestors []Level, s Surface, entry string) bool {
	for _, anc := range ancestors {
		for _, req := range requiredOutputs(anc, s) {
			if globMatch(req, entry) {
				return true
			}
		}
	}
	return false
}

// cueKey writes a key the way CUE source needs it: bare when it is an
// identifier, quoted otherwise.
func cueKey(key string) string {
	// A leading underscore makes a hidden field, not the output's key.
	if strings.HasPrefix(key, "_") {
		return strconv.Quote(key)
	}
	for i, r := range key {
		if !(r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r))) {
			return strconv.Quote(key)
		}
	}
	if key == "" {
		return `""`
	}
	return key
}

// fixedIn reads fixed values from one ancestor compiled with nothing bound,
// where a value from a parameter is still a default or left open.
func fixedIn(name string, unbound cue.Value) fixedFunc {
	return func(path cue.Path) (string, string, bool, bool) {
		value, isDefault, ok := fixedValue(unbound.LookupPath(path))
		return name, value, isDefault, ok
	}
}

// fixedAmong compiles each ancestor with nothing bound, only when a refusal
// needs it, and reports the nearest one that writes the value as a constant.
func fixedAmong(ctx context.Context, ancestors []Level, compile CompileFunc) fixedFunc {
	return func(path cue.Path) (string, string, bool, bool) {
		sels := path.Selectors()
		for _, anc := range ancestors {
			val, _ := compile(ctx, anc.Template+"\n"+SuperField+": _\n")
			if val.Exists() {
				if value, isDefault, ok := fixedValue(val.LookupPath(path)); ok {
					return anc.Name, value, isDefault, true
				}
			}
			// A level that left this output out wrote its own; what lies
			// above it no longer reaches the child.
			if len(sels) > 1 && sels[1].IsString() {
				if d, err := parseDirectives(anc); err == nil && d.dropped(sels[0].String(), sels[1].Unquoted()) {
					break
				}
			}
		}
		return "", "", false, false
	}
}

// fixedValue is the JSON of a value a child cannot change by passing a
// parameter: a constant, or a default written in the template that refers to
// nothing, such as `*"RollingUpdate" | string`. isDefault tells the two apart.
// A struct or list is compared leaf by leaf instead.
func fixedValue(v cue.Value) (value string, isDefault, ok bool) {
	if !v.Exists() {
		return "", false, false
	}
	if d, has := v.Default(); has {
		if refersToAnything(v) {
			return "", false, false
		}
		v, isDefault = d, true
	}
	if !v.IsConcrete() {
		return "", false, false
	}
	if k := v.Kind(); k == cue.StructKind || k == cue.ListKind {
		return "", false, false
	}
	j, err := v.MarshalJSON()
	if err != nil {
		return "", false, false
	}
	return string(j), isDefault, true
}

// refersToAnything reports whether a value's source names anything other than
// a built-in type, such as `parameter.replicas` or `context.name`. Such a
// default is the parent's to set from what it is given.
func refersToAnything(v cue.Value) bool {
	src := v.Source()
	if f, ok := src.(*ast.Field); ok {
		// The source of a field's value is the whole field; its label is not
		// a reference.
		src = f.Value
	}
	if src == nil {
		return true
	}
	found := false
	ast.Walk(src, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && !builtinType[id.Name] {
			found = true
		}
		return !found
	}, nil)
	return found
}

var builtinType = map[string]bool{
	"string": true, "int": true, "float": true, "number": true, "bool": true,
	"bytes": true, "null": true, "_": true, "uint": true,
}

// FixedOutputKind reads the apiVersion and kind a template writes on its output
// as constants, from the template compiled with nothing bound. ok is false when
// either one is missing, or comes from a parameter or the context, since then
// the template does not say what it renders.
func FixedOutputKind(unbound cue.Value) (apiVersion, kind string, ok bool) {
	output := unbound.LookupPath(cue.ParsePath("output"))
	apiVersion, ok = fixedString(output.LookupPath(cue.ParsePath("apiVersion")))
	if !ok {
		return "", "", false
	}
	kind, ok = fixedString(output.LookupPath(cue.ParsePath("kind")))
	if !ok {
		return "", "", false
	}
	return apiVersion, kind, true
}

// fixedString is a string field written as a constant.
func fixedString(v cue.Value) (string, bool) {
	if _, isDefault, ok := fixedValue(v); !ok || isDefault || v.Kind() != cue.StringKind {
		return "", false
	}
	s, err := v.String()
	return s, err == nil
}

// changes reports whether a child value at a path changes the parent's value
// there. A default counts as set: writing even the default pins the field and
// takes it away from the parent's parameter. Over a value the parent fixes, a
// child value that is not concrete, such as `$super.output.metadata.name`, a
// constraint, or at admission a parameter not bound yet, changes nothing the
// merge would not refuse; one that is concrete at render is judged there.
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
	if !parent.IsConcrete() || !child.IsConcrete() {
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
