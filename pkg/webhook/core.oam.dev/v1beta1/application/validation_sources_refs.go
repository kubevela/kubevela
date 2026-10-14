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

package application

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

type sourceReference struct {
	SourceName  string
	Path        string
	FieldPath   *field.Path
	SourceIndex int
	// OpaquePath marks a path the schema validator cannot follow by dotted
	// lookup - one carrying a list index, or a key that itself contains a dot.
	OpaquePath bool
	// Surface is where the read was found: a component, a trait, a policy, a
	// workflow step, or another source's properties (chaining).
	Surface string
}

// pathIsOpaque reports a path the schema validator's dotted lookup cannot
// follow: one carrying a list index, or a key that itself contains a dot.
//
// Both are ordinary reads - `outputs[0].kind`, `labels["platform.io/team"]` -
// and TypeOf checks them properly, against the element type and the map's
// pattern constraint. The coarser HasPath check is skipped for them rather than
// left to reject a valid read, which is what it did to every label key with a
// domain-prefixed name.
func pathIsOpaque(segments []string) bool {
	for _, segment := range segments {
		if strings.Contains(segment, ".") {
			return true
		}
		if segment == "" {
			continue
		}
		digits := true
		for _, r := range segment {
			if r < '0' || r > '9' {
				digits = false
				break
			}
		}
		if digits {
			return true
		}
	}
	return false
}

// effectiveSurfaces maps each source binding to the surfaces it really resolves
// on, following chains.
//
// A binding consumed by a component resolves in a component's context. A binding
// consumed only by another source resolves wherever *that* source is consumed -
// so the surfaces propagate backwards along the chain, and a source used only for
// chaining inherits every surface its consumers are used from.
//
// Chains are acyclic by construction: admission already refuses a source that
// depends on a later one, so a fixpoint converges.
func effectiveSurfaces(refs []sourceReference, bindingAt map[int]string) map[string][]string {
	direct := map[string]map[string]bool{}
	// consumers[a] are the bindings whose own properties read a.
	consumers := map[string][]string{}

	for _, ref := range refs {
		if ref.SourceIndex >= 0 {
			// A read inside spec.sources[i], so the reader is that binding.
			if reader, ok := bindingAt[ref.SourceIndex]; ok {
				consumers[ref.SourceName] = append(consumers[ref.SourceName], reader)
			}
			continue
		}
		if direct[ref.SourceName] == nil {
			direct[ref.SourceName] = map[string]bool{}
		}
		direct[ref.SourceName][ref.Surface] = true
	}

	out := map[string][]string{}
	for name, set := range direct {
		for surface := range set {
			out[name] = append(out[name], surface)
		}
	}
	// Propagate until stable. The graph is small and acyclic; a bounded loop
	// keeps a malformed spec from spinning.
	for i := 0; i < len(refs)+1; i++ {
		changed := false
		for name, readers := range consumers {
			have := map[string]bool{}
			for _, s := range out[name] {
				have[s] = true
			}
			for _, reader := range readers {
				for _, s := range out[reader] {
					if !have[s] {
						have[s] = true
						out[name] = append(out[name], s)
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	for name := range out {
		sort.Strings(out[name])
	}
	return out
}
