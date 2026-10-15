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
	"cuelang.org/go/cue"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/kubevela/pkg/cel/template"

	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// Produced is what one component applies, as component.<name> reads it: its
// workload and its other resources by name. A nil Output, or a resource missing
// from Outputs, declares no kinds.
type Produced struct {
	Output  *openapi3.Schema
	Outputs map[string]*openapi3.Schema
}

// ComponentReadKind is the kind the producer's schema declares for a component
// read, by component name. A placement call does not change what is read, so
// it is passed over.
func ComponentReadKind(produced map[string]Produced, ref template.Reference) (cue.Kind, bool) {
	if !propexpr.IsComponent(ref) {
		return cue.BottomKind, false
	}
	_, path := propexpr.Placement(ref)
	if len(path) < 2 {
		return cue.BottomKind, false
	}
	p, ok := produced[path[0]]
	if !ok {
		return cue.BottomKind, false
	}
	switch path[1] {
	case OutputFieldName:
		if p.Output == nil {
			return cue.BottomKind, false
		}
		return KindAt(p.Output, path[2:])
	case OutputsFieldName:
		if len(path) < 3 || p.Outputs[path[2]] == nil {
			return cue.BottomKind, false
		}
		return KindAt(p.Outputs[path[2]], path[3:])
	}
	return cue.BottomKind, false
}

// EachReadKind is the kind of a read of the loop's iteration: each.index is an
// int, and item, when given, is the kind at a path within each.item.
func EachReadKind(item func(path []string) (cue.Kind, bool), ref template.Reference) (cue.Kind, bool) {
	if ref.Root != propexpr.EachIdent || len(ref.Path) == 0 {
		return cue.BottomKind, false
	}
	switch ref.Path[0] {
	case "index":
		if len(ref.Path) == 1 {
			return cue.IntKind, true
		}
	case "item":
		if item != nil {
			return item(ref.Path[1:])
		}
	}
	return cue.BottomKind, false
}

// EachMayBeAbsent reports whether an each read may find nothing, given item,
// what may be missing within one item of the loop: only each.item's fields can.
func EachMayBeAbsent(item func(path []string) bool, ref template.Reference) bool {
	if ref.Root != propexpr.EachIdent || len(ref.Path) < 2 || ref.Path[0] != "item" || item == nil {
		return false
	}
	return item(ref.Path[1:])
}
