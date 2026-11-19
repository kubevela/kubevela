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
	"sort"
)

// RequiredTag marks an output that nothing extending the definition may leave
// out. On the `outputs` block itself it covers every output in it.
const RequiredTag = "+required"

// requiredOutputs are a level's outputs marked +required, as `field.key`, a
// generated key as a pattern and a marked block as `field.*`. The marker means
// something only where a child could leave an entry out.
func requiredOutputs(level Level, s Surface) []string {
	var out []string
	for _, p := range markedPaths(level, s, RequiredTag) {
		if !droppable[p[0]] {
			continue
		}
		switch len(p) {
		case 1:
			out = append(out, p[0]+".*")
		case 2:
			out = append(out, p.String())
		}
	}
	return out
}

// CheckRequired refuses a level that leaves out an output one of its ancestors
// marked +required. An output a level between them already left out is that
// level's own from then on, so the marker above it no longer reaches.
func CheckRequired(at string, self Level, ancestors []Level, s Surface) error {
	d, err := parseDirectives(self)
	if err != nil {
		return err
	}
	if len(d.drops) == 0 {
		return nil
	}
	cut := map[string]bool{}
	for _, anc := range ancestors {
		for _, req := range requiredOutputs(anc, s) {
			for _, drop := range sortedDrops(d) {
				if cut[drop] || !globMatch(req, drop) {
					continue
				}
				return fmt.Errorf("%s %s leaves out %s, which %s marks %s",
					s.Kind, at, drop, anc.Name, RequiredTag)
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

// sortedDrops are a level's dropped outputs as `field.key`, in a stable order
// so the same template always reports the same one first.
func sortedDrops(d directives) []string {
	set := map[string]bool{}
	cutsOf(d, set)
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
