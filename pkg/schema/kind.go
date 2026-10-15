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
	"math"
	"slices"
	"sort"
	"strconv"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"

	"github.com/kubevela/pkg/cel/template"
)

// preserveUnknownFields marks a value kept as written, whatever its shape.
const preserveUnknownFields = "x-kubernetes-preserve-unknown-fields"

// KindAt is the kind s declares at path, one segment per field, map key or list
// index, when it declares one. A path s does not have, a value of any type and
// anything below one has none: a live object carries more than its schema says.
// Nor does a nullable value, or anything below one, since null is not its kind.
func KindAt(s *openapi3.Schema, path []string) (cue.Kind, bool) {
	for _, seg := range path {
		if s == nil || s.Nullable || s.Extensions[preserveUnknownFields] == true {
			return cue.BottomKind, false
		}
		switch {
		case s.Type.Is(openapi3.TypeArray):
			if _, err := strconv.Atoi(seg); err != nil || s.Items == nil {
				return cue.BottomKind, false
			}
			s = s.Items.Value
		case s.Type.Is(openapi3.TypeObject):
			if p, ok := s.Properties[seg]; ok && p != nil {
				s = p.Value
				continue
			}
			if s.AdditionalProperties.Schema == nil {
				return cue.BottomKind, false
			}
			s = s.AdditionalProperties.Schema.Value
		default:
			return cue.BottomKind, false
		}
	}
	return declaredKind(s)
}

// MayBeAbsentAt reports whether a value s describes may lack path: an object's
// field it declares but does not require, or a map's key, at any segment. A
// list index is taken as present, and a path s does not declare, or that runs
// below a value of any type, is not reported, as for a definition's schema.
func MayBeAbsentAt(s *openapi3.Schema, path []string) bool {
	for _, seg := range path {
		if s == nil {
			return false
		}
		switch {
		case s.Type.Is(openapi3.TypeArray):
			if _, err := strconv.Atoi(seg); err != nil || s.Items == nil {
				return false
			}
			s = s.Items.Value
		case s.Type.Is(openapi3.TypeObject):
			p, ok := s.Properties[seg]
			if !ok || p == nil {
				return s.AdditionalProperties.Schema != nil
			}
			if !slices.Contains(s.Required, seg) {
				return true
			}
			s = p.Value
		default:
			return false
		}
	}
	return false
}

func declaredKind(s *openapi3.Schema) (cue.Kind, bool) {
	if s == nil || s.Nullable || s.Extensions[preserveUnknownFields] == true {
		return cue.BottomKind, false
	}
	switch {
	case s.Type.Is(openapi3.TypeString):
		return cue.StringKind, true
	case s.Type.Is(openapi3.TypeInteger):
		return cue.IntKind, true
	case s.Type.Is(openapi3.TypeNumber):
		return cue.NumberKind, true
	case s.Type.Is(openapi3.TypeBoolean):
		return cue.BoolKind, true
	case s.Type.Is(openapi3.TypeArray):
		return cue.ListKind, true
	case s.Type.Is(openapi3.TypeObject):
		return cue.StructKind, true
	}
	return cue.BottomKind, false
}

// LiteralSchema is the schema of a decoded JSON value as written: a list's
// elements share one schema, which declares a kind only where every element
// agrees on it, and an object's fields are merged the same way across the
// elements that have them, required only where every object element has them.
// A string that is one $( ) expression, and null, are of any type.
func LiteralSchema(v interface{}) *openapi3.Schema {
	switch x := v.(type) {
	case string:
		if parsed, err := template.Parse(x); err == nil && parsed.HasExpr() && parsed.Whole() {
			return &openapi3.Schema{}
		}
		return openapi3.NewStringSchema()
	case bool:
		return openapi3.NewBoolSchema()
	case float64:
		if x == math.Trunc(x) {
			return openapi3.NewIntegerSchema()
		}
		return openapi3.NewFloat64Schema()
	case []interface{}:
		var elem *openapi3.Schema
		for i, e := range x {
			if i == 0 {
				elem = LiteralSchema(e)
				continue
			}
			elem = merge(elem, LiteralSchema(e))
		}
		list := openapi3.NewArraySchema()
		if elem != nil {
			list.Items = openapi3.NewSchemaRef("", elem)
		}
		return list
	case map[string]interface{}:
		obj := openapi3.NewObjectSchema()
		for k, e := range x {
			obj.Properties[k] = openapi3.NewSchemaRef("", LiteralSchema(e))
			obj.Required = append(obj.Required, k)
		}
		sort.Strings(obj.Required)
		return obj
	}
	return &openapi3.Schema{}
}

// merge is the schema two values both fit: the shared kind, objects field by
// field and lists element by element, or any type where they disagree. An
// integer beside a number is a number.
func merge(a, b *openapi3.Schema) *openapi3.Schema {
	ka, oka := declaredKind(a)
	kb, okb := declaredKind(b)
	switch {
	case !oka || !okb:
		return &openapi3.Schema{}
	case ka == cue.IntKind && kb == cue.NumberKind, ka == cue.NumberKind && kb == cue.IntKind:
		return openapi3.NewFloat64Schema()
	case ka != kb:
		return &openapi3.Schema{}
	case ka == cue.StructKind:
		obj := openapi3.NewObjectSchema()
		for k, p := range a.Properties {
			obj.Properties[k] = p
		}
		for k, p := range b.Properties {
			if q, ok := obj.Properties[k]; ok {
				p = openapi3.NewSchemaRef("", merge(q.Value, p.Value))
			}
			obj.Properties[k] = p
		}
		for _, k := range a.Required {
			if slices.Contains(b.Required, k) {
				obj.Required = append(obj.Required, k)
			}
		}
		return obj
	case ka == cue.ListKind:
		list := openapi3.NewArraySchema()
		switch {
		case a.Items == nil:
			list.Items = b.Items
		case b.Items == nil:
			list.Items = a.Items
		default:
			list.Items = openapi3.NewSchemaRef("", merge(a.Items.Value, b.Items.Value))
		}
		return list
	}
	return a
}
