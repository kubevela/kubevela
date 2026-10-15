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
	if len(items) == 0 {
		return nil
	}
	var literal interface{}
	if err := json.Unmarshal(items, &literal); err != nil {
		return nil
	}
	switch v := literal.(type) {
	case []interface{}:
		s := LiteralSchema(v)
		return func(path []string) (cue.Kind, bool) { return KindAt(s, append([]string{"0"}, path...)) }
	case string:
		ref, ok := BareRead(v)
		if !ok || ref.Root == propexpr.EachIdent {
			return nil
		}
		return func(path []string) (cue.Kind, bool) {
			elem := append(append(append([]string{}, ref.Path...), "0"), path...)
			return readKind(template.Reference{Root: ref.Root, Path: elem})
		}
	}
	return nil
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
