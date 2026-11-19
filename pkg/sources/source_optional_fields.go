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

package sources

import (
	"strings"

	"github.com/kubevela/pkg/cel/template"
	"github.com/kubevela/workflow/pkg/cue/process"

	"github.com/oam-dev/kubevela/pkg/definition/celexpr"
	"github.com/oam-dev/kubevela/pkg/definition/propexpr"
)

// OptionalSourceField reports the binding and field behind a required parameter
// the properties do set, through a source read of a field the source's schema
// declares optional. It reports false for any other reason a parameter is
// absent.
//
// A source read contributes only the fields its schema guarantees, so an
// optional one is not among them and the required-field check sees a parameter
// the Application did not provide. It did provide it: the expression is right
// there in the properties. What is missing is the guarantee.
func OptionalSourceField(ctx process.Context, params map[string]interface{}, missing string) (binding, field string, ok bool) {
	segments := strings.Split(missing, ".")
	if ctx == nil || len(segments) < 2 {
		return "", "", false
	}
	schemas := propexpr.CompileSchemas(SchemasForContext(ctx))
	// The read sits on an ancestor of the missing field, and the longest
	// ancestor that is one is what was to provide it.
	for i := len(segments) - 1; i >= 1; i-- {
		raw, found := stringAtPath(params, segments[:i])
		if !found {
			continue
		}
		read, ok := bareSourceRead(raw)
		if !ok {
			continue
		}
		read.Path = append(read.Path, segments[i:]...)
		// The outermost optional field along the path is the one to name:
		// while it may be absent, making anything under it required changes
		// nothing.
		for n := 2; n <= len(read.Path); n++ {
			prefix := template.Reference{Root: read.Root, Path: read.Path[:n]}
			if schemas.DeclaredOptional(prefix) {
				return read.Path[0], strings.Join(read.Path[1:n], "."), true
			}
		}
		return "", "", false
	}
	return "", "", false
}

// stringAtPath is the string a dotted path names in nested properties.
func stringAtPath(params map[string]interface{}, path []string) (string, bool) {
	var node interface{} = params
	for _, segment := range path {
		m, ok := node.(map[string]interface{})
		if !ok {
			return "", false
		}
		if node, ok = m[segment]; !ok {
			return "", false
		}
	}
	s, ok := node.(string)
	return s, ok
}

// bareSourceRead reads a property that is one whole source read and nothing
// else, which is the only shape that contributes a schema's fields.
func bareSourceRead(raw string) (template.Reference, bool) {
	if !template.MayContainExpr(raw) {
		return template.Reference{}, false
	}
	parsed, err := template.Parse(raw)
	if err != nil || !parsed.HasExpr() {
		//nolint:nilerr // a malformed expression is reported by the expression validator
		return template.Reference{}, false
	}
	expr, whole := parsed.SoleExpr()
	if !whole {
		return template.Reference{}, false
	}
	refs, err := celexpr.PropertyReferences(expr)
	if err != nil || len(refs) != 1 {
		return template.Reference{}, false
	}
	ref := refs[0]
	if !propexpr.IsSource(ref) || len(ref.Path) == 0 || ref.String() != expr {
		// Not a bare read: the expression does something with the value, so the
		// schema's fields are not what lands in the parameter.
		return template.Reference{}, false
	}
	return ref, true
}
