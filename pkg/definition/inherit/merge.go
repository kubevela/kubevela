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

	"cuelang.org/go/cue"
)

// mergeSurfaces merges the parent's surface fields onto the child's.
//
// Unification rather than replacement, so new `outputs` keys join the parent's
// and `output` is overlaid field by field. A parent value from a defaulted
// parameter is still a disjunction here, so a child setting it wins; only a
// concrete one conflicts.
func mergeSurfaces(child cue.Value, parent map[string]cue.Value, s Surface, d directives, parentLevel Level, at string) (cue.Value, error) {
	out := child
	for _, field := range s.Fields {
		path := cue.ParsePath(field)
		fromParent, ok := parent[field]
		if !ok {
			continue
		}
		fromParent, err := withoutDropped(fromParent, field, d)
		if err != nil {
			return cue.Value{}, fmt.Errorf("%s %s: leaving out %s's %s: %w", s.Kind, at, parentLevel.Name, field, err)
		}
		fromChild := child.LookupPath(path)
		if !fromChild.Exists() {
			// Nothing of the child's to merge: the parent's stands as it is.
			out = out.FillPath(path, fromParent)
			if err := out.Err(); err != nil {
				return cue.Value{}, fmt.Errorf(
					"%s %s: inheriting %s from %s: %w", s.Kind, at, field, parentLevel.Name, err)
			}
			continue
		}
		merged := fromParent.Unify(fromChild)
		if err := merged.Err(); err != nil {
			return cue.Value{}, fmt.Errorf(
				"%s %s: merging its %s onto %s's: %w\n"+
					"  a conflict here means %s sets that field to a concrete value. "+
					"Pass a different parameter to %s through `%s: properties`",
				s.Kind, at, field, parentLevel.Name, err,
				parentLevel.Name, parentLevel.Name, SuperField)
		}
		out = out.FillPath(path, merged)
		if err := out.Err(); err != nil {
			return cue.Value{}, fmt.Errorf(
				"%s %s: merging %s: %w", s.Kind, at, field, err)
		}
	}
	return out, nil
}

// withoutDropped is a parent's field with the entries the child leaves out
// removed, so the child's own entry of that name, if any, stands alone.
func withoutDropped(v cue.Value, field string, d directives) (cue.Value, error) {
	if len(d.drops[field]) == 0 {
		return v, nil
	}
	iter, err := v.Fields(cue.Optional(true), cue.Definitions(true))
	if err != nil {
		return cue.Value{}, err
	}
	out := v.Context().CompileString("{}")
	for iter.Next() {
		if d.dropped(field, iter.Selector().Unquoted()) {
			continue
		}
		out = out.FillPath(cue.MakePath(iter.Selector()), iter.Value())
	}
	return out, out.Err()
}
