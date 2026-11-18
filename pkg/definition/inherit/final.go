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
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
)

// FinalTag marks a field of a definition's result that nothing extending the
// definition may add to, at any depth below the field and at any level below
// the definition.
const FinalTag = "+final"

// finalPath is a field marked +final, as the labels leading to it from the
// surface. A label built by interpolation is a pattern: its literal parts are
// kept and each interpolated part matches anything.
type finalPath []string

func (p finalPath) String() string { return strings.Join(p, ".") }

// finalPaths reads the +final markers off a template's surface fields.
func finalPaths(level Level, s Surface) []finalPath { return markedPaths(level, s, FinalTag) }

// markedPaths reads the fields of a template's surface carrying a marker, such
// as +final or +required, in their doc comment.
//
// They come from the source rather than the compiled value. A merged value has
// lost its doc comments, and in a shorthand such as
// `securityContext: runAsNonRoot: true` the compiler moves the comment onto the
// innermost field, so the value would protect the wrong field.
func markedPaths(level Level, s Surface, tag string) []finalPath {
	file, err := parser.ParseFile(level.Name+".cue", level.Template, parser.ParseComments)
	if err != nil {
		// A template that will not parse fails later with a better message from
		// the compiler, which knows the kind and the call site.
		return nil
	}
	surface := map[string]bool{}
	for _, f := range s.Fields {
		surface[f] = true
	}
	var out []finalPath
	var walk func(decls []ast.Decl, at finalPath)
	walk = func(decls []ast.Decl, at finalPath) {
		for _, decl := range decls {
			switch d := decl.(type) {
			case *ast.Field:
				label := finalLabel(d.Label)
				if len(at) == 0 && !surface[label] {
					continue
				}
				here := append(append(finalPath{}, at...), label)
				if hasTag(d, tag) {
					out = append(out, here)
				}
				if st, ok := d.Value.(*ast.StructLit); ok {
					walk(st.Elts, here)
				}
			case *ast.Comprehension:
				// A guarded or generated field sits at the same depth as the
				// comprehension that produces it.
				if st, ok := d.Value.(*ast.StructLit); ok {
					walk(st.Elts, at)
				}
			case *ast.EmbedDecl:
				if st, ok := d.Expr.(*ast.StructLit); ok {
					walk(st.Elts, at)
				}
			}
		}
	}
	walk(file.Decls, nil)
	return out
}

// finalLabel is a label as a path element. An interpolated or dynamic label
// becomes a pattern, `*` standing for what is only known at render time.
func finalLabel(label ast.Label) string {
	switch l := label.(type) {
	case *ast.Interpolation:
		var b strings.Builder
		for i, part := range l.Elts {
			if i%2 == 1 {
				b.WriteString("*")
				continue
			}
			lit, ok := part.(*ast.BasicLit)
			if !ok {
				b.WriteString("*")
				continue
			}
			b.WriteString(interpolationText(lit.Value, i == 0, i == len(l.Elts)-1))
		}
		return b.String()
	case *ast.ParenExpr, *ast.ListLit:
		return "*"
	default:
		return labelName(label)
	}
}

// interpolationText is the literal text of one part of an interpolated label.
// The first part carries the opening quote and `\(`, the last the `)` and the
// closing quote, and a middle one both.
func interpolationText(raw string, first, last bool) string {
	if first {
		raw = strings.TrimPrefix(raw, `"`)
	} else {
		raw = strings.TrimPrefix(raw, ")")
	}
	if last {
		raw = strings.TrimSuffix(raw, `"`)
	} else {
		raw = strings.TrimSuffix(raw, `\(`)
	}
	return raw
}

// hasTag reports whether a field's doc comment carries the tag on a line of its
// own, optionally followed by a reason.
func hasTag(f *ast.Field, tag string) bool {
	for _, cg := range ast.Comments(f) {
		if !cg.Doc {
			continue
		}
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if text == tag || strings.HasPrefix(text, tag+" ") {
				return true
			}
		}
	}
	return false
}

// CheckFinal refuses a level that adds to a field one of its ancestors marked
// +final. own is the level's own compiled value, before anything was merged
// onto it, so whatever it holds under a surface field it wrote itself.
//
// A marker on an output holds only while that output is still merged: a level
// that leaves it out with `$inherit` writes its own from then on, and nothing
// above it reaches the levels below through it.
func CheckFinal(at string, self Level, own cue.Value, ancestors []Level, s Surface) error {
	d, err := parseDirectives(self)
	if err != nil {
		return err
	}
	cut := map[string]bool{}
	addCuts := func(d directives) {
		for field, keys := range d.drops {
			for key := range keys {
				cut[field+"."+key] = true
			}
		}
	}
	addCuts(d)
	for _, anc := range ancestors {
		for _, p := range finalPaths(anc, s) {
			if hit, ok := touches(own, p, cut); ok {
				return fmt.Errorf("%s %s adds to %s, which %s marks %s",
					s.Kind, at, hit, anc.Name, FinalTag)
			}
		}
		ad, err := parseDirectives(anc)
		if err != nil {
			return err
		}
		addCuts(ad)
	}
	return nil
}

// touches reports whether v holds anything at or below p, and the concrete path
// it found, a pattern element matched against v's own labels. An output listed
// in cut was left out on the way up and is the level's own, so it is skipped.
func touches(v cue.Value, p finalPath, cut map[string]bool) (string, bool) {
	var walk func(v cue.Value, rest finalPath, seen []string) (string, bool)
	walk = func(v cue.Value, rest finalPath, seen []string) (string, bool) {
		if !v.Exists() {
			return "", false
		}
		if len(seen) == 2 && cut[seen[0]+"."+seen[1]] {
			return "", false
		}
		if len(rest) == 0 {
			return strings.Join(seen, "."), true
		}
		if !strings.Contains(rest[0], "*") {
			return walk(v.LookupPath(cue.MakePath(cue.Str(rest[0]))), rest[1:], append(seen, rest[0]))
		}
		iter, err := v.Fields()
		if err != nil {
			return "", false
		}
		for iter.Next() {
			if !iter.Selector().IsString() {
				continue
			}
			label := iter.Selector().Unquoted()
			if !globMatch(rest[0], label) {
				continue
			}
			if hit, ok := walk(iter.Value(), rest[1:], append(seen, label)); ok {
				return hit, true
			}
		}
		return "", false
	}
	return walk(v, p, nil)
}

// globMatch matches a label against a pattern in which `*` stands for any run
// of characters, and nothing else is special.
func globMatch(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		last := i == len(parts)-1
		if last {
			return strings.HasSuffix(s, parts[i])
		}
		idx := strings.Index(s, parts[i])
		if idx < 0 {
			return false
		}
		s = s[idx+len(parts[i]):]
	}
	return s == ""
}
