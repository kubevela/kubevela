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
func mergeSurfaces(child cue.Value, parent map[string]cue.Value, s Surface, d directives, self, parentLevel Level) (cue.Value, error) {
	out := child
	for _, field := range s.Fields {
		if !d.inherits(field) {
			continue
		}
		path := cue.ParsePath(field)
		fromParent, ok := parent[field]
		if !ok {
			continue
		}
		fromChild := child.LookupPath(path)
		if !fromChild.Exists() {
			// Nothing of the child's to merge: the parent's stands as it is.
			out = out.FillPath(path, fromParent)
			if err := out.Err(); err != nil {
				return cue.Value{}, fmt.Errorf(
					"%s %s: inheriting %s from %s: %w", s.Kind, self.Name, field, parentLevel.Name, err)
			}
			continue
		}
		merged := fromParent.Unify(fromChild)
		if err := merged.Err(); err != nil {
			return cue.Value{}, fmt.Errorf(
				"%s %s: merging its %s onto %s's: %w\n"+
					"  a conflict here means %s sets that field to a concrete value. "+
					"Pass a different parameter to %s, or take the field over with `%s: {%s: false}`",
				s.Kind, self.Name, field, parentLevel.Name, err,
				parentLevel.Name, parentLevel.Name, InheritField, field)
		}
		out = out.FillPath(path, merged)
		if err := out.Err(); err != nil {
			return cue.Value{}, fmt.Errorf(
				"%s %s: merging %s: %w", s.Kind, self.Name, field, err)
		}
	}
	return out, nil
}
