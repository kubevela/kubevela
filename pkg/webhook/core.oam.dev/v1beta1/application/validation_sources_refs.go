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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
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

// withSurface stamps the surface onto each collected reference.
func withSurface(refs []sourceReference, surface string) []sourceReference {
	for i := range refs {
		refs[i].Surface = surface
	}
	return refs
}

// collectSourceRefs returns every `source` read an expression makes within a
// properties blob, as the reference records the validation loop consumes.
//
// The loop is mechanism-agnostic: declared-ness, chaining order, surface and
// consumableFrom are properties of *reading a source*, not of how the read was
// spelled. Only this collector knew about the directive form, which is what let
// it be removed without losing a single one of those checks.
func collectSourceRefs(raw *runtime.RawExtension, basePath *field.Path, sourceIndex int) ([]sourceReference, field.ErrorList) {
	if raw == nil || len(raw.Raw) == 0 {
		return nil, nil
	}
	var decoded interface{}
	if err := json.Unmarshal(raw.Raw, &decoded); err != nil {
		return nil, field.ErrorList{field.Invalid(basePath, string(raw.Raw),
			fmt.Sprintf("invalid properties: %v", err))}
	}

	var refs []sourceReference
	for _, lf := range flattenLeafPaths(raw.Raw, basePath) {
		text, ok := lf.literal.(string)
		if !ok {
			continue
		}
		parsed, err := propexpr.Parse(text)
		if err != nil || !parsed.HasExpr() {
			continue
		}
		for _, fragment := range parsed.Fragments {
			if !fragment.IsExpr() {
				continue
			}
			reads, rerr := celexpr.PropertyReferences(fragment.Expr)
			if rerr != nil {
				// Syntax errors are reported by validateExpressions with a
				// better message; do not report them twice.
				continue
			}
			for _, read := range reads {
				// The binding name is all a reference needs. A whole-binding
				// read - $(source.cfg) rather than $(source.cfg.host) - has
				// nothing after it, and skipping those skipped every check the
				// loop makes: the binding did not have to be declared, come
				// earlier in a chain, or allow the surface reading it.
				if !read.IsSource() || len(read.Path) < 1 {
					continue
				}
				refs = append(refs, sourceReference{
					SourceName: read.Path[0],
					Path:       strings.Join(read.Path[1:], "."),
					// Whether the dotted form round-trips has to be decided here,
					// while the segments are still separate. `labels["a.b/c"]`
					// joins to `labels.a.b/c`, which no longer says where the key
					// began - and a list index joins to a segment the schema has
					// no field for at all.
					OpaquePath:  pathIsOpaque(read.Path[1:]),
					FieldPath:   lf.fieldPath,
					SourceIndex: sourceIndex,
				})
			}
		}
	}
	return refs, nil
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
