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

package propexpr

import (
	"strconv"

	"cuelang.org/go/cue"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"
)

// Schemas are the binding schemas a set of reads is judged against, compiled
// once, since a source's schema is CUE whatever expressions are written in.
type Schemas struct{ compiled map[string]cue.Value }

// CompileSchemas compiles binding schemas given as CUE text. A schema that will
// not compile is left out: the binding then has nothing to judge against, and
// the definition's own validation reports why.
func CompileSchemas(texts map[string]string) Schemas {
	compiled := map[string]cue.Value{}
	for name, text := range texts {
		v := newContext().CompileString(text)
		if v.Err() != nil {
			continue
		}
		compiled[name] = v
	}
	return Schemas{compiled: compiled}
}

// CanBeAbsent reports whether a read could be missing at render, whatever
// guards it: a default is needed exactly where the schema does not promise the
// value, an optional field or a key of an open map. A declared non-optional
// field is always present.
func (s Schemas) CanBeAbsent(ref template.Reference) (bool, error) {
	return canBeAbsent(ref, s.compiled)
}

// Kind is the kind the schema declares for a read, when it declares one: the
// binding's schema for a source, the registry for context.
func (s Schemas) Kind(ref template.Reference) (cue.Kind, bool) {
	var v cue.Value
	var path []string
	switch ref.Root {
	case SourceIdent:
		if len(ref.Path) == 0 {
			return cue.BottomKind, false
		}
		schema, ok := s.compiled[ref.Path[0]]
		if !ok {
			return cue.BottomKind, false
		}
		v, path = schema, ref.Path[1:]
	case ContextIdent:
		if len(ref.Path) == 0 {
			return cue.BottomKind, false
		}
		field, ok := contextField(ref.Path[0])
		if !ok {
			return cue.BottomKind, false
		}
		v, path = field, ref.Path[1:]
	default:
		return cue.BottomKind, false
	}
	for _, seg := range path {
		next, ok := schemaChild(v, seg)
		if !ok {
			return cue.BottomKind, false
		}
		v = next
	}
	return v.IncompleteKind(), true
}

// schemaChild steps into a schema by one read segment: a field, optional or
// not, a key of an open map, or a list index.
func schemaChild(v cue.Value, seg string) (cue.Value, bool) {
	if v.IncompleteKind() == cue.ListKind {
		i, err := strconv.Atoi(seg)
		if err != nil {
			return cue.Value{}, false
		}
		if elem := v.LookupPath(cue.MakePath(cue.Index(i))); elem.Exists() {
			return elem, true
		}
		elem := v.LookupPath(cue.MakePath(cue.AnyIndex))
		return elem, elem.Exists()
	}
	if iter, err := v.Fields(cue.Optional(true)); err == nil {
		for iter.Next() {
			if iter.Selector().Unquoted() == seg {
				return iter.Value(), true
			}
		}
	}
	pattern := v.LookupPath(cue.MakePath(cue.AnyString))
	return pattern, pattern.Exists()
}

// canBeAbsent judges a read against whatever declares its root: a source against
// its binding's schema, context against the registry.
func canBeAbsent(ref template.Reference, schemas map[string]cue.Value) (bool, error) {
	switch ref.Root {
	case SourceIdent:
		return sourceCanBeAbsent(ref, schemas)
	case ContextIdent:
		// A field the registry declares is judged by its declared shape, so a
		// nested read such as context.clusterVersion.major is as certain as
		// context.namespace.
		if len(ref.Path) < 2 {
			// A plain field is always supplied, even when its value is empty.
			return false, nil
		}
		field, ok := contextField(ref.Path[0])
		if !ok {
			// Not a field any surface offers. The read is refused elsewhere, by
			// whichever surface check owns it.
			return false, nil
		}
		return celengine.OptionalPath(field, ref.Path[1:])
	case ComponentIdent:
		// A missing value makes the reader wait rather than fail, so no default
		// is owed.
		return false, nil
	default:
		// An undeclared root is refused by root validation.
		return false, nil
	}
}

func sourceCanBeAbsent(ref template.Reference, schemas map[string]cue.Value) (bool, error) {
	if len(ref.Path) == 0 {
		// A bare `source` names no binding. Root validation refuses it; there is
		// nothing here to judge against.
		return false, nil
	}
	binding := ref.Path[0]
	schema, ok := schemas[binding]
	if !ok {
		// Nothing to judge against. The read is checked elsewhere - TypeOf
		// reports an unknown source - so this is not the place to complain.
		return false, nil
	}
	return celengine.OptionalPath(schema, ref.Path[1:])
}

// contextField returns the declared value of a context field, from whichever
// surface offers it. Surfaces differ in which fields they carry, never in what a
// field is, so the first match is the answer.
func contextField(name string) (cue.Value, bool) {
	for _, surface := range SurfaceNames() {
		if v, ok := ContextFor(surface).FieldValue(name); ok {
			return v, true
		}
	}
	return cue.Value{}, false
}
