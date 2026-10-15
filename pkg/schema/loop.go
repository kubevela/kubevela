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
	"encoding/json"
	"strings"

	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	celengine "github.com/kubevela/pkg/cel"
	"github.com/kubevela/pkg/cel/template"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// LoopItemKind is the kind at a path within one item of a forEach loop, given its
// items: a literal list is typed as written, and one expression that is exactly
// a read is typed by what that read declares one element down. Anything else, a
// computed list or items from a workflow variable, declares nothing.
func LoopItemKind(items []byte, readKind func(template.Reference) (cue.Kind, bool)) func([]string) (cue.Kind, bool) {
	literal, read, ok := loopItems(items)
	switch {
	case !ok:
		return nil
	case literal != nil:
		return func(path []string) (cue.Kind, bool) { return KindAt(literal, append([]string{"0"}, path...)) }
	}
	return func(path []string) (cue.Kind, bool) { return readKind(elementRead(read, path)) }
}

// LoopItemMayBeAbsent reports whether a path within one item of a forEach loop
// may be missing, given its items: in a literal list, a field some object items
// leave out, and for one expression that is exactly a read, what mayBeAbsent
// says one element down. Items LoopItemKind types nothing for say nothing.
func LoopItemMayBeAbsent(items []byte, mayBeAbsent func(template.Reference) bool) func([]string) bool {
	literal, read, ok := loopItems(items)
	switch {
	case !ok:
		return nil
	case literal != nil:
		return func(path []string) bool { return MayBeAbsentAt(literal, append([]string{"0"}, path...)) }
	}
	return func(path []string) bool { return mayBeAbsent(elementRead(read, path)) }
}

// loopItems is a forEach's items as a literal list's schema, or as the read
// they consist of; ok is false for items that are neither.
func loopItems(items []byte) (literal *openapi3.Schema, read template.Reference, ok bool) {
	if len(items) == 0 {
		return nil, read, false
	}
	var v interface{}
	if err := json.Unmarshal(items, &v); err != nil {
		return nil, read, false
	}
	switch x := v.(type) {
	case []interface{}:
		return LiteralSchema(x), read, true
	case string:
		ref, ok := BareRead(x)
		if !ok || ref.Root == propexpr.EachIdent {
			return nil, read, false
		}
		return nil, ref, true
	}
	return nil, read, false
}

// elementRead is the read of path within one element of the list read reads.
func elementRead(read template.Reference, path []string) template.Reference {
	return template.Reference{Root: read.Root, Path: append(append(append([]string{}, read.Path...), "0"), path...)}
}

// BareRead is the read a value consists of when it is one $( ) expression doing
// nothing but read, such as $(source.cfg.regions).
func BareRead(value string) (template.Reference, bool) {
	parsed, err := template.Parse(value)
	if err != nil || !parsed.HasExpr() || !parsed.Whole() {
		return template.Reference{}, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return template.Reference{}, false
	}
	plan, err := celexpr.Vela.PlanJSON(raw)
	if err != nil {
		return template.Reference{}, false
	}
	var reads []celengine.Read
	for _, root := range celexpr.Roots {
		reads = append(reads, plan.Reads(root.Name)...)
	}
	if len(reads) != 1 {
		return template.Reference{}, false
	}
	ref := reads[0].Reference
	expr := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "$("), ")"))
	if ref.Defaulted || expr != ref.String() {
		return template.Reference{}, false
	}
	return ref, true
}
